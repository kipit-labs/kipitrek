package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/delivery/http/middleware"
	"github.com/kipit-labs/kipitrek/internal/domain"
)

type authService struct {
	AuthService
	err        error
	loggedOut  *string
	deletedFor *uuid.UUID
}

var sessionExpiry = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

func (s authService) Login(_ context.Context, email, password string) (string, *domain.Session, error) {
	if s.err != nil {
		return "", nil, s.err
	}
	return "tok-" + email, &domain.Session{ExpiresAt: sessionExpiry}, nil
}

func (s authService) Logout(_ context.Context, token string) error {
	*s.loggedOut = token
	return s.err
}

func (s authService) DeleteAccount(_ context.Context, userID uuid.UUID, _ string) error {
	if s.err != nil {
		return s.err
	}
	*s.deletedFor = userID
	return nil
}

func TestLoginSetsASafeSessionCookie(t *testing.T) {
	for _, secure := range []bool{false, true} {
		rec := call(NewAuthHandler(authService{}, secure).Login, http.MethodPost, "/", "/", jsonBody(`{"email":"ana@x.y","password":"pw"}`), nil)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status %d", rec.Code)
		}
		c := sessionCookie(rec)
		switch {
		case c == nil:
			t.Fatal("no session cookie")
		case c.Value != "tok-ana@x.y":
			t.Errorf("value %q", c.Value)
		case !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/":
			t.Errorf("cookie readable by scripts or other sites: %+v", c)
		case c.Secure != secure:
			t.Errorf("Secure %v, want %v", c.Secure, secure)
		case !c.Expires.Equal(sessionExpiry):
			t.Errorf("expires %v, want the session's %v", c.Expires, sessionExpiry)
		}
	}
}

func TestFailedLoginSetsNoCookie(t *testing.T) {
	rec := call(NewAuthHandler(authService{err: domain.ErrInvalidCredentials}, true).Login, http.MethodPost, "/", "/", jsonBody(`{"email":"a@b.c","password":"x"}`), nil)
	wantError(t, rec, http.StatusUnauthorized, "invalid_credentials")
	if c := sessionCookie(rec); c != nil {
		t.Errorf("cookie set on failed login: %+v", c)
	}
}

func TestLogoutEndsTheSessionAndClearsTheCookie(t *testing.T) {
	var token string
	h := NewAuthHandler(authService{loggedOut: &token}, true)

	rec := call(h.Logout, http.MethodPost, "/", "/", nil, nil, "Cookie", middleware.SessionCookie+"=abc")
	if rec.Code != http.StatusNoContent || token != "abc" {
		t.Fatalf("status %d, logged out %q", rec.Code, token)
	}
	if c := sessionCookie(rec); c == nil || c.Value != "" || c.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %+v", c)
	}

	token = "untouched"
	if rec := call(h.Logout, http.MethodPost, "/", "/", nil, nil); rec.Code != http.StatusNoContent || token != "untouched" {
		t.Errorf("signed-out logout: status %d, logged out %q", rec.Code, token)
	}
}

func TestDeleteAccount(t *testing.T) {
	u := &domain.User{ID: uuid.New()}
	var deleted uuid.UUID
	rec := call(NewAuthHandler(authService{deletedFor: &deleted}, true).DeleteAccount, http.MethodDelete, "/", "/", jsonBody(`{"password":"pw"}`), u)
	if rec.Code != http.StatusNoContent || deleted != u.ID {
		t.Fatalf("status %d, deleted %v, want %v", rec.Code, deleted, u.ID)
	}
	if c := sessionCookie(rec); c == nil || c.MaxAge >= 0 {
		t.Errorf("cookie not cleared: %+v", c)
	}

	rec = call(NewAuthHandler(authService{err: domain.ErrInvalidCredentials}, true).DeleteAccount, http.MethodDelete, "/", "/", jsonBody(`{"password":"bad"}`), u)
	wantError(t, rec, http.StatusUnauthorized, "invalid_credentials")
	if c := sessionCookie(rec); c != nil {
		t.Errorf("cookie cleared although the account still exists: %+v", c)
	}
}
