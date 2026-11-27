//go:build integration

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/kipit-labs/kipitrek/internal/config"
	"github.com/kipit-labs/kipitrek/internal/domain"
	"github.com/kipit-labs/kipitrek/internal/infrastructure/postgres"
)

// These tests drive the real application (wire) over HTTP against PostGIS:
// only email is faked. They are the safety net for refactors: a change that
// keeps behavior keeps them green without editing them.

// inbox keeps the last email sent to each address.
type inbox struct {
	mu   sync.Mutex
	last map[string]string
}

func (b *inbox) Send(_ context.Context, to, _, body string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.last[to] = body
	return nil
}

var tokenInLink = regexp.MustCompile(`token=([^\s&"]+)`)

func (b *inbox) token(t *testing.T, to string) string {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	m := tokenInLink.FindStringSubmatch(b.last[to])
	if m == nil {
		t.Fatalf("no link emailed to %s: %q", to, b.last[to])
	}
	tok, _ := url.QueryUnescape(m[1])
	return tok
}

type testApp struct {
	url  string
	mail *inbox
	db   *gorm.DB
}

func newTestApp(t *testing.T) *testApp {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	db, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err := postgres.Migrate(db); err != nil {
		t.Fatal(err)
	}
	// An admin that already exists keeps the accounts below from being the
	// first one (made admin), whatever else the database holds.
	seed := &domain.User{ID: uuid.New(), Email: uuid.NewString() + "@test.local", PasswordHash: "x", IsAdmin: true, CreatedAt: time.Now()}
	if err := postgres.NewUserRepository(db).Create(ctx, seed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Delete(&postgres.UserModel{}, "id = ?", seed.ID) })

	mail := &inbox{last: map[string]string{}}
	a, err := wire(&config.Config{
		SessionTTL:          time.Hour,
		MaxUploadMB:         1,
		RegistrationEnabled: true,
		AppURL:              "http://kipitrek.test",
	}, db, mail)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.router)
	t.Cleanup(srv.Close)
	return &testApp{url: srv.URL, mail: mail, db: db}
}

// client is one browser: it keeps its own session cookie.
type client struct {
	t     *testing.T
	app   *testApp
	http  *http.Client
	email string
	id    string
}

func (a *testApp) visitor(t *testing.T) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, app: a, http: &http.Client{Jar: jar}}
}

// do sends a request and decodes a JSON answer into out (when not nil).
func (c *client) do(method, path string, body any, out any, wantStatus int) []byte {
	c.t.Helper()
	var r io.Reader
	ct := ""
	switch b := body.(type) {
	case nil:
	case *multipartForm:
		r, ct = &b.buf, b.ct
	default:
		raw, _ := json.Marshal(b)
		r, ct = bytes.NewReader(raw), "application/json"
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.app.url+path, r)
	if err != nil {
		c.t.Fatal(err)
	}
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: %v: %s", method, path, err, raw)
		}
	}
	return raw
}

func (c *client) errorCode(method, path string, body any, wantStatus int) string {
	c.t.Helper()
	var e struct{ Code string }
	c.do(method, path, body, &e, wantStatus)
	return e.Code
}

type multipartForm struct {
	buf bytes.Buffer
	ct  string
}

func gpxForm(t *testing.T, field, file string) *multipartForm {
	t.Helper()
	data, err := os.ReadFile("../../internal/infrastructure/gpx/testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	f := &multipartForm{}
	mw := multipart.NewWriter(&f.buf)
	w, _ := mw.CreateFormFile(field, file)
	_, _ = w.Write(data)
	_ = mw.Close()
	f.ct = mw.FormDataContentType()
	return f
}

const password = "correct horse battery"

// signUp registers, verifies the email from the link sent, and is signed in.
func (a *testApp) signUp(t *testing.T) *client {
	t.Helper()
	c := a.visitor(t)
	c.email = uuid.NewString() + "@test.local"
	c.do(http.MethodPost, "/api/auth/register", map[string]string{"email": c.email, "password": password, "language": "en"}, nil, http.StatusCreated)
	if code := c.errorCode(http.MethodPost, "/api/auth/login", map[string]string{"email": c.email, "password": password}, http.StatusForbidden); code != "email_not_verified" {
		t.Fatalf("unverified login: code %q", code)
	}
	c.do(http.MethodPost, "/api/auth/verify", map[string]string{"token": a.mail.token(t, c.email)}, nil, http.StatusNoContent)
	var me struct{ ID, Email string }
	c.do(http.MethodGet, "/api/auth/me", nil, &me, http.StatusOK)
	if me.Email != c.email {
		t.Fatalf("signed in as %q, want %q", me.Email, c.email)
	}
	c.id = me.ID
	// Straight in the database: a failed test may have changed the password.
	t.Cleanup(func() { a.db.Delete(&postgres.UserModel{}, "email = ?", c.email) })
	return c
}

type hikeJSON struct {
	ID           string
	Name         string
	DistanceM    float64
	StartedAt    *time.Time
	Planned      bool
	Interactions *struct{ Kudos, Comments int }
}

func (c *client) upload(file, query string) hikeJSON {
	c.t.Helper()
	var out struct {
		Results []struct {
			Hike *hikeJSON
			Code string
		}
	}
	c.do(http.MethodPost, "/api/hikes"+query, gpxForm(c.t, "files", file), &out, http.StatusOK)
	if len(out.Results) != 1 || out.Results[0].Hike == nil {
		c.t.Fatalf("upload %s: %+v", file, out.Results)
	}
	return *out.Results[0].Hike
}

func TestAPIAccountLifecycle(t *testing.T) {
	a := newTestApp(t)
	ana := a.signUp(t)

	ana.do(http.MethodPatch, "/api/me", map[string]string{"name": "Ana"}, nil, http.StatusOK)

	// Signed out, the session no longer works; signing in again does.
	ana.do(http.MethodPost, "/api/auth/logout", nil, nil, http.StatusNoContent)
	ana.do(http.MethodGet, "/api/auth/me", nil, nil, http.StatusUnauthorized)
	if code := ana.errorCode(http.MethodPost, "/api/auth/login", map[string]string{"email": ana.email, "password": "wrong"}, http.StatusUnauthorized); code != "invalid_credentials" {
		t.Errorf("wrong password: code %q", code)
	}
	ana.do(http.MethodPost, "/api/auth/login", map[string]string{"email": ana.email, "password": password}, nil, http.StatusNoContent)

	// A password reset by email signs out the other devices.
	other := a.visitor(t)
	other.do(http.MethodPost, "/api/auth/login", map[string]string{"email": ana.email, "password": password}, nil, http.StatusNoContent)
	ana.do(http.MethodPost, "/api/auth/password/forgot", map[string]string{"email": ana.email}, nil, http.StatusNoContent)
	const newPassword = "another horse battery"
	ana.do(http.MethodPost, "/api/auth/password/reset", map[string]string{"token": a.mail.token(t, ana.email), "password": newPassword}, nil, http.StatusNoContent)
	other.do(http.MethodGet, "/api/auth/me", nil, nil, http.StatusUnauthorized)
	ana.do(http.MethodPost, "/api/auth/password", map[string]string{"currentPassword": newPassword, "password": password}, nil, http.StatusNoContent)

	// Deleting the account takes the hikes with it and closes sign-in.
	h := ana.upload("track.gpx", "")
	ana.do(http.MethodDelete, "/api/me", map[string]string{"password": password}, nil, http.StatusNoContent)
	a.visitor(t).do(http.MethodGet, "/api/hikes/"+h.ID, nil, nil, http.StatusNotFound)
	ana.do(http.MethodPost, "/api/auth/login", map[string]string{"email": ana.email, "password": password}, nil, http.StatusUnauthorized)
}

func TestAPIHikes(t *testing.T) {
	a := newTestApp(t)
	ana := a.signUp(t)

	h := ana.upload("track.gpx", "")
	if h.Name != "Col de la Croix" || h.DistanceM <= 0 || h.StartedAt == nil || h.Planned {
		t.Fatalf("imported %+v", h)
	}
	var list []hikeJSON
	ana.do(http.MethodGet, "/api/hikes", nil, &list, http.StatusOK)
	if len(list) != 1 || list[0].ID != h.ID {
		t.Fatalf("list %+v", list)
	}
	ana.do(http.MethodGet, "/api/hikes/"+h.ID+"/profile", nil, nil, http.StatusOK)
	if gpx := ana.do(http.MethodGet, "/api/hikes/"+h.ID+"/gpx", nil, nil, http.StatusOK); !bytes.Contains(gpx, []byte("<trkpt")) {
		t.Errorf("downloaded GPX %s", gpx)
	}
	ana.do(http.MethodPatch, "/api/hikes/"+h.ID, map[string]any{"name": "Croix", "labels": []string{"alps"}}, nil, http.StatusOK)
	var labels []string
	ana.do(http.MethodGet, "/api/labels", nil, &labels, http.StatusOK)
	if len(labels) != 1 || labels[0] != "alps" {
		t.Errorf("labels %v", labels)
	}

	// A planned route keeps no times until it is walked.
	plan := ana.upload("route.gpx", "?planned=true")
	if !plan.Planned || plan.StartedAt != nil {
		t.Fatalf("planned %+v", plan)
	}
	var done hikeJSON
	ana.do(http.MethodPost, "/api/hikes/"+plan.ID+"/done", gpxForm(t, "file", "track.gpx"), &done, http.StatusOK)
	if done.Planned || done.StartedAt == nil {
		t.Errorf("marked done %+v", done)
	}

	var bad struct{ Results []struct{ Code string } }
	ana.do(http.MethodPost, "/api/hikes", gpxForm(t, "files", "invalid.gpx"), &bad, http.StatusOK)
	if len(bad.Results) != 1 || bad.Results[0].Code != "invalid_gpx" {
		t.Errorf("invalid upload %+v", bad.Results)
	}

	raw := ana.do(http.MethodGet, "/api/hikes/export", nil, nil, http.StatusOK)
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || len(zr.File) != 2 {
		t.Fatalf("export: %v, %d files", err, len(zr.File))
	}

	var deleted struct{ Deleted int }
	ana.do(http.MethodPost, "/api/hikes/delete", map[string]any{"ids": []string{h.ID, plan.ID}}, &deleted, http.StatusOK)
	if deleted.Deleted != 2 {
		t.Errorf("deleted %d", deleted.Deleted)
	}
	ana.do(http.MethodGet, "/api/hikes", nil, &list, http.StatusOK)
	if len(list) != 0 {
		t.Errorf("left %+v", list)
	}
}

// Who sees a hike follows its owner's visibility and friendships.
func TestAPIVisibilityAndFriends(t *testing.T) {
	a := newTestApp(t)
	ana, bo := a.signUp(t), a.signUp(t)
	anon := a.visitor(t)
	h := ana.upload("track.gpx", "")
	path := "/api/hikes/" + h.ID

	sees := func(who *client, want bool) {
		t.Helper()
		status := http.StatusNotFound
		if want {
			status = http.StatusOK
		}
		who.do(http.MethodGet, path, nil, nil, status)
	}
	sees(ana, true)
	sees(bo, false)
	sees(anon, false)

	ana.do(http.MethodPatch, "/api/me", map[string]string{"visibility": "public"}, nil, http.StatusOK)
	sees(bo, true)
	sees(anon, true)

	ana.do(http.MethodPatch, "/api/me", map[string]string{"visibility": "friends"}, nil, http.StatusOK)
	sees(bo, false)
	sees(anon, false)

	// A request alone shows nothing; once accepted, friends see the hike.
	var rel struct{ Relation string }
	bo.do(http.MethodPut, "/api/friends/"+ana.id, nil, &rel, http.StatusOK)
	sees(bo, false)
	ana.do(http.MethodPut, "/api/friends/"+bo.id, nil, &rel, http.StatusOK)
	if rel.Relation != "friends" {
		t.Fatalf("relation %q", rel.Relation)
	}
	sees(bo, true)
	sees(anon, false)

	// Friends interact; only the owner edits.
	bo.do(http.MethodPut, path+"/kudos", nil, nil, http.StatusNoContent)
	bo.do(http.MethodPost, path+"/comments", map[string]string{"body": "nice one"}, nil, http.StatusCreated)
	var got hikeJSON
	ana.do(http.MethodGet, path, nil, &got, http.StatusOK)
	if got.Interactions == nil || got.Interactions.Kudos != 1 || got.Interactions.Comments != 1 {
		t.Errorf("interactions %+v", got.Interactions)
	}
	bo.do(http.MethodPatch, path, map[string]string{"name": "mine now"}, nil, http.StatusNotFound)
	bo.do(http.MethodDelete, path, nil, nil, http.StatusNotFound)

	var feed struct{ Hikes []hikeJSON }
	bo.do(http.MethodGet, "/api/feed", nil, &feed, http.StatusOK)
	if len(feed.Hikes) != 1 || feed.Hikes[0].ID != h.ID {
		t.Errorf("feed %+v", feed.Hikes)
	}

	bo.do(http.MethodDelete, "/api/friends/"+ana.id, nil, nil, http.StatusNoContent)
	sees(bo, false)
}
