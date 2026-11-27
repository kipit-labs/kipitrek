package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

func TestRegisterLoginAuthenticateLogout(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	svc, sessions := e.svc, e.sessions

	u := e.registerVerified(t, "  Alice@Example.com ", "password123")
	if u.Email != "alice@example.com" {
		t.Errorf("email not normalized: %q", u.Email)
	}

	token, _, err := svc.Login(ctx, "ALICE@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sessions.byHash[token]; ok {
		t.Error("raw token must not be stored")
	}

	got, err := svc.Authenticate(ctx, token)
	if err != nil || got.ID != u.ID {
		t.Fatalf("authenticate: %v, %v", got, err)
	}

	if err := svc.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("after logout err = %v", err)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	svc := e.svc
	e.registerVerified(t, "a@b.co", "password123")
	if _, _, err := svc.Login(ctx, "a@b.co", "wrong-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("wrong password err = %v", err)
	}
	if _, _, err := svc.Login(ctx, "nobody@b.co", "password123"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("unknown user err = %v", err)
	}
}

func TestAuthenticateExpired(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	svc, sessions := e.svc, e.sessions
	e.registerVerified(t, "a@b.co", "password123")
	for h := range sessions.byHash {
		delete(sessions.byHash, h)
	}
	token, _, err := svc.Login(ctx, "a@b.co", "password123")
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := svc.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("expired err = %v", err)
	}
	if len(sessions.byHash) != 0 {
		t.Error("expired session should be deleted")
	}
}

func TestBannedCannotSignIn(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	u := e.registerVerified(t, "a@b.co", "password123")
	token, _, err := e.svc.Login(ctx, "a@b.co", "password123")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	u.BannedAt, u.BanReason = &now, "spam"

	var be *domain.BannedError
	if _, _, err := e.svc.Login(ctx, "a@b.co", "password123"); !errors.As(err, &be) || be.Reason != "spam" {
		t.Errorf("login err = %v", err)
	}
	// The reason is only shown to whoever knows the password.
	if _, _, err := e.svc.Login(ctx, "a@b.co", "wrong-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("wrong password err = %v", err)
	}
	if _, err := e.svc.Authenticate(ctx, token); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("authenticate err = %v", err)
	}

	if err := e.svc.RequestPasswordReset(ctx, "a@b.co"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.ResetPassword(ctx, e.mailer.lastToken(t), "password456"); !errors.As(err, &be) {
		t.Errorf("reset err = %v", err)
	}
	if u.PasswordHash != "h:password123" {
		t.Error("banned user changed their password")
	}
}

func TestSessions(t *testing.T) {
	e := newTestEnv(t)
	e.registerVerified(t, "a@b.co", "password123")
	laptop := domain.WithClient(context.Background(), domain.Client{UserAgent: "Firefox", IP: "10.0.0.1"})
	phone := domain.WithClient(context.Background(), domain.Client{UserAgent: "Safari", IP: "10.0.0.2"})
	lt, _, err := e.svc.Login(laptop, "a@b.co", "password123")
	if err != nil {
		t.Fatal(err)
	}
	pt, _, err := e.svc.Login(phone, "a@b.co", "password123")
	if err != nil {
		t.Fatal(err)
	}
	u, err := e.svc.Authenticate(laptop, lt)
	if err != nil {
		t.Fatal(err)
	}

	list, current, err := e.svc.Sessions(laptop, u.ID, lt)
	if err != nil {
		t.Fatal(err)
	}
	// The verification link started a session too.
	if len(list) != 3 {
		t.Fatalf("sessions = %d, want 3", len(list))
	}
	var laptopID, phoneID uuid.UUID
	for _, s := range list {
		switch s.UserAgent {
		case "Firefox":
			laptopID = s.ID
			if s.IP != "10.0.0.1" {
				t.Errorf("laptop ip = %q", s.IP)
			}
		case "Safari":
			phoneID = s.ID
		}
	}
	if current != laptopID || laptopID == uuid.Nil {
		t.Errorf("current = %v, laptop = %v", current, laptopID)
	}

	var ve *domain.ValidationError
	if err := e.svc.RevokeSession(laptop, u.ID, laptopID, lt); !errors.As(err, &ve) {
		t.Errorf("revoke current err = %v", err)
	}
	if err := e.svc.RevokeSession(laptop, u.ID, phoneID, lt); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Authenticate(phone, pt); !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("revoked session err = %v", err)
	}
	if err := e.svc.RevokeOtherSessions(laptop, u.ID, lt); err != nil {
		t.Fatal(err)
	}
	if list, _, _ := e.svc.Sessions(laptop, u.ID, lt); len(list) != 1 || list[0].ID != laptopID {
		t.Errorf("after revoking others = %+v", list)
	}
}

func TestAuthenticateTouchesStaleSession(t *testing.T) {
	e := newTestEnv(t)
	e.registerVerified(t, "a@b.co", "password123")
	token, sess, err := e.svc.Login(context.Background(), "a@b.co", "password123")
	if err != nil {
		t.Fatal(err)
	}
	start := sess.LastUsedAt
	later := domain.WithClient(context.Background(), domain.Client{IP: "10.0.0.9"})

	e.svc.now = func() time.Time { return start.Add(time.Minute) }
	if _, err := e.svc.Authenticate(later, token); err != nil {
		t.Fatal(err)
	}
	if sess.LastUsedAt != start {
		t.Error("touched a fresh session")
	}
	e.svc.now = func() time.Time { return start.Add(10 * time.Minute) }
	if _, err := e.svc.Authenticate(later, token); err != nil {
		t.Fatal(err)
	}
	if !sess.LastUsedAt.Equal(start.Add(10*time.Minute)) || sess.IP != "10.0.0.9" {
		t.Errorf("session = %+v", sess)
	}
}
