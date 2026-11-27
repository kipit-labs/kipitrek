package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/kipit-labs/kipitrek/internal/delivery/http/dto"
	"github.com/kipit-labs/kipitrek/internal/delivery/http/middleware"
	"github.com/kipit-labs/kipitrek/internal/domain"
)

// call serves one request to fn mounted at pattern, signed in as u when u is
// not nil, and returns the recorded response.
func call(fn http.HandlerFunc, method, pattern, target string, body io.Reader, u *domain.User, header ...string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	var h http.Handler = fn
	if u != nil {
		h = middleware.RequireAuth(fakeAuth{u})(h)
	}
	r.Method(method, pattern, h)

	req := httptest.NewRequest(method, target, body)
	if u != nil {
		req.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: "token"})
	}
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func jsonBody(s string) io.Reader { return strings.NewReader(s) }

// wantError checks the response is a JSON error with this status and code.
func wantError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) dto.Error {
	t.Helper()
	var e dto.Error
	if rec.Code != status {
		t.Fatalf("status %d, want %d (body %s)", rec.Code, status, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body is not a JSON error: %s", rec.Body)
	}
	if e.Code != code {
		t.Fatalf("code %q, want %q", e.Code, code)
	}
	return e
}

func sessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == middleware.SessionCookie {
			return c
		}
	}
	return nil
}

type fakeAuth struct{ user *domain.User }

func (a fakeAuth) Authenticate(context.Context, string) (*domain.User, error) { return a.user, nil }
