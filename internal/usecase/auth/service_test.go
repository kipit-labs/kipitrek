package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

var linkRe = regexp.MustCompile(`https://app\.test/(?:verify|reset-password)\?token=(\S+)`)

func TestRegisterValidation(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)

	var ve *domain.ValidationError
	if _, err := svc.Register(ctx, "not-an-email", "password123", ""); !errors.As(err, &ve) || ve.Field != "email" {
		t.Errorf("bad email err = %v", err)
	}
	if _, err := svc.Register(ctx, "a@b.co", "short", ""); !errors.As(err, &ve) || ve.Field != "password" {
		t.Errorf("short password err = %v", err)
	}
	if _, err := svc.Register(ctx, "a@b.co", "password123", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, "A@b.co", "password123", ""); !errors.Is(err, domain.ErrEmailTaken) {
		t.Errorf("duplicate err = %v", err)
	}
}

func TestEmailVerification(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)

	u, err := e.svc.Register(ctx, "a@b.co", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.mailer.sent) != 1 || e.mailer.sent[0].to != "a@b.co" {
		t.Fatalf("sent = %+v", e.mailer.sent)
	}
	if len(e.sessions.byHash) != 0 {
		t.Error("register must not start a session")
	}
	if _, _, err := e.svc.Login(ctx, "a@b.co", "password123"); !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Errorf("unverified login err = %v", err)
	}
	// A wrong password must not reveal that the account is unverified.
	if _, _, err := e.svc.Login(ctx, "a@b.co", "wrong-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("unverified wrong password err = %v", err)
	}

	token := e.mailer.lastToken(t)
	if _, ok := e.verifications.byUser[u.ID]; !ok || e.verifications.byUser[u.ID].TokenHash == token {
		t.Error("only the token hash must be stored")
	}
	sessToken, _, err := e.svc.VerifyEmail(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := e.svc.Authenticate(ctx, sessToken); err != nil || got.ID != u.ID {
		t.Fatalf("authenticate after verify: %v, %v", got, err)
	}
	if _, _, err := e.svc.VerifyEmail(ctx, token); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("reused token err = %v", err)
	}
	if _, _, err := e.svc.Login(ctx, "a@b.co", "password123"); err != nil {
		t.Errorf("verified login err = %v", err)
	}
	if len(e.mailer.sent) != 1 {
		t.Errorf("sent %d emails, want 1", len(e.mailer.sent))
	}
}

func TestVerifyEmailInvalidToken(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	if _, err := e.svc.Register(ctx, "a@b.co", "password123", ""); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"", "nope"} {
		if _, _, err := e.svc.VerifyEmail(ctx, token); !errors.Is(err, domain.ErrInvalidToken) {
			t.Errorf("token %q err = %v", token, err)
		}
	}

	token := e.mailer.lastToken(t)
	e.svc.now = func() time.Time { return time.Now().Add(VerificationTTL + time.Minute) }
	if _, _, err := e.svc.VerifyEmail(ctx, token); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("expired token err = %v", err)
	}
}

func TestUnverifiedLoginSendsLinkOncePerTTL(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	u, err := e.svc.Register(ctx, "a@b.co", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	first := e.mailer.lastToken(t)

	login := func() {
		t.Helper()
		if _, _, err := e.svc.Login(ctx, "a@b.co", "password123"); !errors.Is(err, domain.ErrEmailNotVerified) {
			t.Fatalf("unverified login err = %v", err)
		}
	}

	// The link from sign-up is still valid: no new email.
	login()
	login()
	if _, _, err := e.svc.Login(ctx, "a@b.co", "wrong-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v", err)
	}
	if len(e.mailer.sent) != 1 {
		t.Fatalf("sent %d emails while the link is valid, want 1", len(e.mailer.sent))
	}

	// Once it expires, the next attempt sends a new link, which replaces it.
	e.svc.now = func() time.Time { return time.Now().Add(VerificationTTL + time.Minute) }
	login()
	login()
	if len(e.mailer.sent) != 2 {
		t.Fatalf("sent %d emails after expiry, want 2", len(e.mailer.sent))
	}
	if _, _, err := e.svc.VerifyEmail(ctx, first); !errors.Is(err, domain.ErrInvalidToken) {
		t.Errorf("superseded token err = %v", err)
	}
	if _, _, err := e.svc.VerifyEmail(ctx, e.mailer.lastToken(t)); err != nil {
		t.Errorf("new token err = %v", err)
	}
	if got, _ := e.svc.users.GetByID(ctx, u.ID); got.EmailVerifiedAt == nil {
		t.Error("user not verified")
	}
}

// Accounts without any link (e.g. created before verification existed) get
// one on their first sign-in attempt.
func TestUnverifiedLoginWithoutLinkSendsOne(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	if _, err := e.svc.Register(ctx, "a@b.co", "password123", ""); err != nil {
		t.Fatal(err)
	}
	e.verifications.byUser = map[uuid.UUID]*domain.EmailVerification{}
	e.mailer.sent = nil

	if _, _, err := e.svc.Login(ctx, "a@b.co", "password123"); !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Fatalf("err = %v", err)
	}
	if len(e.mailer.sent) != 1 || e.mailer.sent[0].to != "a@b.co" {
		t.Fatalf("sent = %+v", e.mailer.sent)
	}
}

func TestFailedSendIsRetriedOnNextLogin(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	e.mailer.err = errors.New("smtp down")
	if _, err := e.svc.Register(ctx, "a@b.co", "password123", ""); err != nil {
		t.Fatal(err)
	}
	if len(e.verifications.byUser) != 0 {
		t.Fatal("undelivered link must not be kept")
	}

	e.mailer.err = nil
	if _, _, err := e.svc.Login(ctx, "a@b.co", "password123"); !errors.Is(err, domain.ErrEmailNotVerified) {
		t.Fatalf("err = %v", err)
	}
	if len(e.mailer.sent) != 1 {
		t.Fatalf("sent %d emails, want 1", len(e.mailer.sent))
	}
}

func TestBannedCannotVerify(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)
	u, err := e.svc.Register(ctx, "a@b.co", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	u.BannedAt, u.BanReason = &now, "spam"
	var be *domain.BannedError
	if _, _, err := e.svc.VerifyEmail(ctx, e.mailer.lastToken(t)); !errors.As(err, &be) {
		t.Errorf("verify err = %v", err)
	}
	if len(e.sessions.byHash) != 0 {
		t.Error("banned user got a session")
	}
}

func TestWithoutEmail(t *testing.T) {
	ctx := context.Background()
	e := newTestEnvWithoutEmail(t)
	if e.svc.EmailEnabled() || e.svc.RequiresVerifiedEmail() {
		t.Fatal("email should be off")
	}
	u, err := e.svc.Register(ctx, "a@b.co", "password123", "")
	if err != nil {
		t.Fatal(err)
	}
	if u.EmailVerifiedAt != nil || len(e.verifications.byUser) != 0 {
		t.Error("no verification without a mail server")
	}
	// Unverified accounts sign in when nothing could verify them.
	if _, _, err := e.svc.Login(ctx, "a@b.co", "password123"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if err := e.svc.RequestPasswordReset(ctx, "a@b.co"); !errors.Is(err, domain.ErrEmailDisabled) {
		t.Errorf("reset request err = %v", err)
	}
	// Asking to email an invite hands back the link instead.
	_, link, emailed, err := e.svc.Invite(ctx, "c@d.co", "", "", true)
	if err != nil || emailed || link == "" {
		t.Fatalf("invite = %q, %v, %v", link, emailed, err)
	}
	if _, _, err := e.svc.ResetPassword(ctx, inviteToken(t, link), "password456"); err != nil {
		t.Fatal(err)
	}
}

func TestEmailsFollowTheUserLanguage(t *testing.T) {
	ctx := context.Background()
	e := newTestEnv(t)

	u, err := e.svc.Register(ctx, "a@b.co", "password123", domain.LanguageFrench)
	if err != nil {
		t.Fatal(err)
	}
	if u.Language != domain.LanguageFrench {
		t.Errorf("language = %q", u.Language)
	}
	if got := e.mailer.sent[0].subject; got != "Confirmez votre adresse e-mail" {
		t.Errorf("verification subject = %q", got)
	}

	// Unknown or missing languages fall back to English.
	if _, err := e.svc.Register(ctx, "c@d.co", "password123", "xx"); err != nil {
		t.Fatal(err)
	}
	if got := e.mailer.sent[1].subject; got != "Confirm your email address" {
		t.Errorf("fallback subject = %q", got)
	}
}
