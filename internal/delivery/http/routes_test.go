package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/delivery/http/middleware"
	"github.com/kipit-labs/kipitrek/internal/domain"
)

type access int

const (
	public access = iota // anyone, signed in or not
	user                 // signed in
	admin                // signed in as an admin
)

// routeAccess is the security contract of the API: every route declares who
// may call it. A new route fails TestEveryRouteDeclaresItsAccess until it is
// listed here.
var routeAccess = map[string]access{
	"GET /api/health":                public,
	"GET /api/config":                public,
	"POST /api/auth/register":        public,
	"POST /api/auth/login":           public,
	"POST /api/auth/logout":          public,
	"POST /api/auth/verify":          public,
	"POST /api/auth/password/forgot": public,
	"POST /api/auth/password/reset":  public,

	"GET /api/hikes/{id}":              public,
	"GET /api/hikes/{id}/profile":      public,
	"GET /api/hikes/{id}/gpx":          public,
	"GET /api/hikes/{id}/card.png":     public,
	"GET /api/hikes/{id}/summits":      public,
	"GET /api/hikes/{id}/comments":     public,
	"GET /api/users/{id}":              public,
	"GET /api/users/{id}/hikes":        public,
	"GET /api/users/{id}/hikes/tracks": public,

	"GET /api/auth/me":                             user,
	"POST /api/auth/password":                      user,
	"PATCH /api/me":                                user,
	"DELETE /api/me":                               user,
	"GET /api/me/sessions":                         user,
	"DELETE /api/me/sessions":                      user,
	"DELETE /api/me/sessions/{id}":                 user,
	"GET /api/hikes":                               user,
	"POST /api/hikes":                              user,
	"GET /api/hikes/tracks":                        user,
	"GET /api/hikes/export":                        user,
	"POST /api/hikes/export":                       user,
	"POST /api/hikes/delete":                       user,
	"GET /api/labels":                              user,
	"GET /api/tiles":                               user,
	"GET /api/summits":                             user,
	"GET /api/feed":                                user,
	"PATCH /api/hikes/{id}":                        user,
	"POST /api/hikes/{id}/done":                    user,
	"DELETE /api/hikes/{id}":                       user,
	"GET /api/hikes/{id}/similar":                  user,
	"PUT /api/hikes/{id}/kudos":                    user,
	"DELETE /api/hikes/{id}/kudos":                 user,
	"POST /api/hikes/{id}/comments":                user,
	"DELETE /api/hikes/{id}/comments/{commentId}":  user,
	"PUT /api/hikes/{id}/participants/{userId}":    user,
	"DELETE /api/hikes/{id}/participants/{userId}": user,
	"GET /api/friends":                             user,
	"PUT /api/friends/{id}":                        user,
	"DELETE /api/friends/{id}":                     user,

	"GET /api/admin/users":                              admin,
	"GET /api/admin/users/{id}":                         admin,
	"GET /api/admin/users/{id}/sessions":                admin,
	"DELETE /api/admin/users/{id}/sessions":             admin,
	"DELETE /api/admin/users/{id}/sessions/{sessionId}": admin,
	"POST /api/admin/users":                             admin,
	"POST /api/admin/users/{id}/password-link":          admin,
	"PUT /api/admin/users/{id}/verified":                admin,
	"DELETE /api/admin/users/{id}/verified":             admin,
	"DELETE /api/admin/users/{id}/invite":               admin,
	"PUT /api/admin/users/{id}/ban":                     admin,
	"DELETE /api/admin/users/{id}/ban":                  admin,
	"PUT /api/admin/users/{id}/admin":                   admin,
	"DELETE /api/admin/users/{id}/admin":                admin,
}

// roleAuth signs in "user" as a plain user and "admin" as an admin.
type roleAuth struct{}

func (roleAuth) Authenticate(_ context.Context, token string) (*domain.User, error) {
	switch token {
	case "user":
		return &domain.User{ID: uuid.New()}, nil
	case "admin":
		return &domain.User{ID: uuid.New(), IsAdmin: true}, nil
	}
	return nil, domain.ErrUnauthorized
}

func apiRoutes(t *testing.T, h http.Handler) []string {
	t.Helper()
	var routes []string
	err := chi.Walk(h.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/api/") {
			routes = append(routes, method+" "+route)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return routes
}

func TestEveryRouteDeclaresItsAccess(t *testing.T) {
	routes := apiRoutes(t, NewRouter(Deps{Authenticator: roleAuth{}}))
	seen := map[string]bool{}
	for _, r := range routes {
		seen[r] = true
		if _, ok := routeAccess[r]; !ok {
			t.Errorf("%s is not in routeAccess: declare who may call it", r)
		}
	}
	for r := range routeAccess {
		if !seen[r] {
			t.Errorf("routeAccess lists %s, which the router no longer serves", r)
		}
	}
}

// The handlers are nil: a request that got past the middleware would panic
// into a 500, so only the middleware can answer these.
func TestRoutesRejectWhoMayNotCallThem(t *testing.T) {
	h := NewRouter(Deps{Authenticator: roleAuth{}})
	for _, r := range apiRoutes(t, h) {
		method, pattern, _ := strings.Cut(r, " ")
		path := strings.NewReplacer("{id}", uuid.NewString(), "{commentId}", uuid.NewString(),
			"{userId}", uuid.NewString(), "{sessionId}", uuid.NewString()).Replace(pattern)

		call := func(session string) int {
			req := httptest.NewRequest(method, path, nil)
			if session != "" {
				req.AddCookie(&http.Cookie{Name: middleware.SessionCookie, Value: session})
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			return rec.Code
		}
		switch routeAccess[r] {
		case user:
			for _, s := range []string{"", "expired"} {
				if got := call(s); got != http.StatusUnauthorized {
					t.Errorf("%s with session %q: %d, want 401", r, s, got)
				}
			}
		case admin:
			if got := call(""); got != http.StatusUnauthorized {
				t.Errorf("%s signed out: %d, want 401", r, got)
			}
			if got := call("user"); got != http.StatusForbidden {
				t.Errorf("%s as a plain user: %d, want 403", r, got)
			}
		}
	}
}

func TestCrossSiteWritesAreRejected(t *testing.T) {
	h := NewRouter(Deps{Authenticator: roleAuth{}})
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/auth/login", nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("cross-site %s: %d, want 403", method, rec.Code)
		}
	}
}
