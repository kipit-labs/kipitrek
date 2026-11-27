package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

// Every domain error reaches the client as a stable status and code, wrapped
// or not; anything else is a 500 that does not leak its message.
func TestWriteErrorMapsDomainErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{&domain.ValidationError{Field: "name", Message: "too long", Code: "name_too_long"}, http.StatusBadRequest, "name_too_long"},
		{&domain.BannedError{Reason: "spam"}, http.StatusForbidden, "banned"},
		{domain.ErrEmailDisabled, http.StatusServiceUnavailable, "email_disabled"},
		{domain.ErrRegistrationClosed, http.StatusForbidden, "registration_closed"},
		{domain.ErrNotFound, http.StatusNotFound, "not_found"},
		{domain.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials"},
		{domain.ErrEmailNotVerified, http.StatusForbidden, "email_not_verified"},
		{domain.ErrInvalidToken, http.StatusBadRequest, "invalid_token"},
		{domain.ErrUnauthorized, http.StatusUnauthorized, "unauthorized"},
		{domain.ErrEmailTaken, http.StatusConflict, "email_taken"},
		{domain.ErrConflict, http.StatusConflict, "conflict"},
		{domain.ErrInvalidGPX, http.StatusBadRequest, "invalid_gpx"},
		{errors.New("pq: connection refused"), http.StatusInternalServerError, "internal"},
	}
	for _, c := range cases {
		for _, err := range []error{c.err, fmt.Errorf("context: %w", c.err)} {
			rec := httptest.NewRecorder()
			writeError(rec, httptest.NewRequest(http.MethodGet, "/", nil), err)
			t.Run(err.Error(), func(t *testing.T) { wantError(t, rec, c.status, c.code) })
		}
	}

	rec := httptest.NewRecorder()
	writeError(rec, httptest.NewRequest(http.MethodGet, "/", nil), errors.New("pq: password=hunter2"))
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Errorf("internal error leaks its message: %s", rec.Body)
	}

	rec = httptest.NewRecorder()
	writeError(rec, httptest.NewRequest(http.MethodGet, "/", nil), &domain.BannedError{Reason: "spam"})
	if e := wantError(t, rec, http.StatusForbidden, "banned"); e.Reason != "spam" {
		t.Errorf("ban reason %q, want spam", e.Reason)
	}
}

func TestDecodeJSONRejectsBadBodies(t *testing.T) {
	for name, body := range map[string]string{
		"malformed":     `{"email":`,
		"unknown field": `{"email":"a@b.c","password":"x","admin":true}`,
		"too large":     `{"email":"` + strings.Repeat("a", 1<<20) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := call(NewAuthHandler(authService{}, false).Login, http.MethodPost, "/", "/", jsonBody(body), nil)
			wantError(t, rec, http.StatusBadRequest, "invalid_body")
		})
	}
}

func TestInvalidIDIsNotFound(t *testing.T) {
	rec := call(NewSummitHandler(nil).OnHike, http.MethodGet, "/hikes/{id}", "/hikes/nope", nil, nil)
	wantError(t, rec, http.StatusNotFound, "not_found")
}

func TestGPXFilename(t *testing.T) {
	for in, want := range map[string]string{
		"Lac Blanc":        "Lac Blanc.gpx",
		`a/b\c:d*e?"<>|`:   "a_b_c_d_e_____.gpx",
		"  ":               "hike.gpx",
		"tab\there":        "tab_here.gpx",
		"../../etc/passwd": ".._.._etc_passwd.gpx",
	} {
		if got := gpxFilename(in); got != want {
			t.Errorf("gpxFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
