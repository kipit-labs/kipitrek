package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/domain"
	"github.com/kipit-labs/kipitrek/internal/usecase/admin"
)

// accountService records which updates ran, failing the one named in fail.
type accountService struct {
	calls *[]string
	fail  string
}

func (s accountService) record(name string, u *domain.User) (*domain.User, error) {
	*s.calls = append(*s.calls, name)
	if name == s.fail {
		return nil, &domain.ValidationError{Field: name, Code: "invalid_" + name}
	}
	return u, nil
}

func (s accountService) UpdateName(_ context.Context, id uuid.UUID, name string) (*domain.User, error) {
	return s.record("name", &domain.User{ID: id, Name: name})
}

func (s accountService) UpdateVisibility(_ context.Context, id uuid.UUID, v domain.Visibility) (*domain.User, error) {
	return s.record("visibility", &domain.User{ID: id, Visibility: v})
}

func (s accountService) UpdateLanguage(_ context.Context, id uuid.UUID, l domain.Language) (*domain.User, error) {
	return s.record("language", &domain.User{ID: id, Language: l})
}

func TestAccountUpdate(t *testing.T) {
	u := &domain.User{ID: uuid.New()}
	cases := []struct {
		body, fail string
		status     int
		calls      []string
	}{
		{body: `{}`, status: http.StatusBadRequest},
		{body: `{"name":"Ana"}`, status: http.StatusOK, calls: []string{"name"}},
		{body: `{"name":"Ana","visibility":"public","language":"fr"}`, status: http.StatusOK, calls: []string{"name", "visibility", "language"}},
		// The first failure stops the rest.
		{body: `{"name":"Ana","visibility":"x","language":"fr"}`, fail: "visibility", status: http.StatusBadRequest, calls: []string{"name", "visibility"}},
	}
	for _, c := range cases {
		var calls []string
		rec := call(NewAccountHandler(accountService{calls: &calls, fail: c.fail}).Update, http.MethodPatch, "/", "/", jsonBody(c.body), u)
		if rec.Code != c.status || !slices.Equal(calls, c.calls) {
			t.Errorf("%s: status %d, calls %v; want %d, %v", c.body, rec.Code, calls, c.status, c.calls)
		}
	}
}

type adminService struct {
	AdminService
	got *[]any
}

func (s adminService) ListUsers(_ context.Context, q string, page int) (*admin.UserPage, error) {
	*s.got = []any{q, page}
	return &admin.UserPage{Total: 1}, nil
}

func (s adminService) Invite(_ context.Context, email, _ string, lang domain.Language, send bool) (*domain.User, string, bool, error) {
	*s.got = []any{email, lang, send}
	return &domain.User{ID: uuid.New(), Email: email}, "https://link", send, nil
}

func (s adminService) Ban(_ context.Context, actor, target uuid.UUID, reason string) error {
	*s.got = []any{actor, target, reason}
	return nil
}

func (s adminService) SetAdmin(_ context.Context, actor, target uuid.UUID, on bool) error {
	*s.got = []any{actor, target, on}
	if actor == target {
		return &domain.ValidationError{Code: "cannot_demote_self"}
	}
	return nil
}

func TestAdminUsersPaging(t *testing.T) {
	me := &domain.User{ID: uuid.New(), IsAdmin: true}
	for target, want := range map[string][]any{
		"/":             {"", 1},
		"/?page=0":      {"", 1},
		"/?page=-4":     {"", 1},
		"/?page=x":      {"", 1},
		"/?page=3&q=an": {"an", 3},
	} {
		var got []any
		rec := call(NewAdminHandler(adminService{got: &got}, nil).Users, http.MethodGet, "/", target, nil, me)
		var page struct{ Page, PageSize int }
		_ = json.Unmarshal(rec.Body.Bytes(), &page)
		if rec.Code != http.StatusOK || !slices.Equal(got, want) || page.Page != want[1] || page.PageSize != admin.PageSize {
			t.Errorf("%s: status %d, service got %v, page %+v", target, rec.Code, got, page)
		}
	}
}

// Invitees get the inviting admin's language until they pick one.
func TestAdminInviteUsesTheAdminsLanguage(t *testing.T) {
	me := &domain.User{ID: uuid.New(), IsAdmin: true, Language: domain.Language("fr")}
	var got []any
	rec := call(NewAdminHandler(adminService{got: &got}, nil).CreateUser, http.MethodPost, "/", "/", jsonBody(`{"email":"bo@x.y","name":"Bo","sendEmail":true}`), me)
	if rec.Code != http.StatusCreated || !slices.Equal(got, []any{"bo@x.y", domain.Language("fr"), true}) {
		t.Fatalf("status %d, service got %v", rec.Code, got)
	}
	var out struct {
		InviteLink string `json:"inviteLink"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.InviteLink != "https://link" {
		t.Errorf("invite link missing: %s", rec.Body)
	}
}

// Admin actions name the acting admin, so the service can refuse self-harm.
func TestAdminActionsPassTheActor(t *testing.T) {
	me := &domain.User{ID: uuid.New(), IsAdmin: true}
	target := uuid.New()
	var got []any
	h := NewAdminHandler(adminService{got: &got}, nil)

	rec := call(h.Ban, http.MethodPut, "/users/{id}/ban", "/users/"+target.String()+"/ban", jsonBody(`{"reason":"spam"}`), me)
	if rec.Code != http.StatusNoContent || !slices.Equal(got, []any{me.ID, target, "spam"}) {
		t.Errorf("ban: status %d, service got %v", rec.Code, got)
	}

	rec = call(h.Demote, http.MethodDelete, "/users/{id}/admin", "/users/"+me.ID.String()+"/admin", nil, me)
	wantError(t, rec, http.StatusBadRequest, "cannot_demote_self")
	if !slices.Equal(got, []any{me.ID, me.ID, false}) {
		t.Errorf("demote: service got %v", got)
	}
}
