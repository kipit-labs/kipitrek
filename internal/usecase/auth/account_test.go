package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

func TestDeleteAccount(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	u := e.registerVerified(t, "a@b.co", "password123")

	var ve *domain.ValidationError
	if err := e.svc.DeleteAccount(ctx, u.ID, "wrong-password"); !errors.As(err, &ve) || ve.Field != "password" {
		t.Fatalf("wrong password err = %v", err)
	}
	if _, err := e.svc.users.GetByID(ctx, u.ID); err != nil {
		t.Fatalf("user deleted despite wrong password: %v", err)
	}

	if err := e.svc.DeleteAccount(ctx, u.ID, "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.users.GetByID(ctx, u.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("after delete err = %v", err)
	}
	if _, _, err := e.svc.Login(ctx, "a@b.co", "password123"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("login after delete err = %v", err)
	}
}

func TestFirstAccountIsAdmin(t *testing.T) {
	e := newTestEnv(t)
	first := e.registerVerified(t, "a@b.co", "password123")
	second := e.registerVerified(t, "c@d.co", "password123")
	if !first.IsAdmin || second.IsAdmin {
		t.Errorf("admins: first %v, second %v", first.IsAdmin, second.IsAdmin)
	}
}

func TestRegistrationClosed(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	e.svc.registration = false

	if open, err := e.svc.RegistrationOpen(ctx); err != nil || !open {
		t.Fatalf("open before first account = %v, %v", open, err)
	}
	// The first account can always sign up, so the instance gets an admin.
	u, err := e.svc.Register(ctx, "a@b.co", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if !u.IsAdmin {
		t.Error("first account should be admin")
	}
	if open, err := e.svc.RegistrationOpen(ctx); err != nil || open {
		t.Fatalf("open after first account = %v, %v", open, err)
	}
	if _, err := e.svc.Register(ctx, "c@d.co", "password123", ""); !errors.Is(err, domain.ErrRegistrationClosed) {
		t.Errorf("second register err = %v", err)
	}
}

func TestInvite(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	e.registerVerified(t, "admin@b.co", "password123")
	e.svc.registration = false
	sent := len(e.mailer.sent)

	u, link, emailed, err := e.svc.Invite(ctx, " Friend@B.co ", "Friend", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if emailed || len(e.mailer.sent) != sent {
		t.Error("invite emailed without being asked")
	}
	if u.Email != "friend@b.co" || u.Name != "Friend" || u.IsAdmin || u.EmailVerifiedAt != nil || u.InvitedAt == nil {
		t.Errorf("invited user = %+v", u)
	}
	if _, _, _, err := e.svc.Invite(ctx, "friend@b.co", "", "", false); !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("duplicate invite err = %v", err)
	}

	// A new link replaces the first one.
	link2, emailed, err := e.svc.PasswordLink(ctx, u.ID, true)
	if err != nil || !emailed {
		t.Fatalf("reissue = %v, %v", emailed, err)
	}
	if !strings.Contains(e.mailer.sent[len(e.mailer.sent)-1].body, link2) {
		t.Error("emailed invite lacks the link")
	}
	if _, _, err := e.svc.ResetPassword(ctx, inviteToken(t, link), "password456"); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("replaced link err = %v", err)
	}
	if _, _, err := e.svc.ResetPassword(ctx, inviteToken(t, link2), "password456"); err != nil {
		t.Fatal(err)
	}
	if u.EmailVerifiedAt == nil || u.InvitedAt != nil {
		t.Errorf("following the invite should verify the email and accept it: %+v", u)
	}
	if _, _, err := e.svc.Login(ctx, "friend@b.co", "password456"); err != nil {
		t.Errorf("login after invite: %v", err)
	}

	// Once accepted, an admin's link is a password reset, not an invite.
	reset, _, err := e.svc.PasswordLink(ctx, u.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := url.Parse(reset)
	if r.Query().Get("invite") != "" {
		t.Errorf("reset link %q should not be an invite", reset)
	}
	if _, _, err := e.svc.ResetPassword(ctx, r.Query().Get("token"), "password789"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.Login(ctx, "friend@b.co", "password789"); err != nil {
		t.Errorf("login after admin reset: %v", err)
	}
}

func TestPasswordLinkRefusesBanned(t *testing.T) {
	e := newTestEnv(t)
	u := e.registerVerified(t, "a@b.co", "password123")
	now := time.Now()
	u.BannedAt = &now
	var ve *domain.ValidationError
	if _, _, err := e.svc.PasswordLink(context.Background(), u.ID, false); !errors.As(err, &ve) {
		t.Errorf("err = %v", err)
	}
}

func TestLastAdminCannotDeleteAccount(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	admin := e.registerVerified(t, "a@b.co", "password123")
	other := e.registerVerified(t, "c@d.co", "password123")

	var ve *domain.ValidationError
	if err := e.svc.DeleteAccount(ctx, admin.ID, "password123"); !errors.As(err, &ve) || ve.Field != "account" {
		t.Fatalf("err = %v", err)
	}
	other.IsAdmin = true
	if err := e.svc.DeleteAccount(ctx, admin.ID, "password123"); err != nil {
		t.Fatal(err)
	}
}
