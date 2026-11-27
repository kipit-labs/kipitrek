package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

const (
	MinPasswordLength = 8
	// VerificationTTL is how long an email verification link stays valid. No
	// new link is sent while one is still valid.
	VerificationTTL = 24 * time.Hour
	// PasswordResetTTL is how long a password reset link stays valid.
	PasswordResetTTL = time.Hour
	// PasswordResetCooldown is the minimum delay between two reset emails to
	// the same account, so the form cannot be used to flood an inbox.
	PasswordResetCooldown = time.Minute
	// InviteTTL is how long the link of an account created by an admin stays
	// valid.
	InviteTTL = 7 * 24 * time.Hour
	// AdminResetTTL is how long a password link an admin hands out stays valid.
	AdminResetTTL = 24 * time.Hour
	// sessionTouchInterval is how stale a session's last use may get before
	// a request records it again, so browsing does not write on every call.
	sessionTouchInterval = 5 * time.Minute
	maxUserAgentLength   = 512
)

type Service struct {
	users         domain.UserRepository
	sessions      domain.SessionRepository
	verifications domain.EmailVerificationRepository
	resets        domain.PasswordResetRepository
	hasher        domain.PasswordHasher
	mailer        domain.Mailer
	ttl           time.Duration
	appURL        string
	registration  bool
	now           func() time.Time
	dummyHash     string
}

// NewService builds the auth service. appURL is the public base URL of the
// app, used in the links it emails. Unless registration is enabled, only the
// first account can sign up; admins invite the others. mailer is nil when no
// mail server is configured: emails are then never required to be verified,
// and links are only handed out by admins.
func NewService(
	users domain.UserRepository,
	sessions domain.SessionRepository,
	verifications domain.EmailVerificationRepository,
	resets domain.PasswordResetRepository,
	hasher domain.PasswordHasher,
	mailer domain.Mailer,
	ttl time.Duration,
	appURL string,
	registration bool,
) (*Service, error) {
	// Used to keep login timing constant when the email is unknown.
	dummy, err := hasher.Hash("dummy-password-for-timing")
	if err != nil {
		return nil, err
	}
	return &Service{
		users:         users,
		sessions:      sessions,
		verifications: verifications,
		resets:        resets,
		hasher:        hasher,
		mailer:        mailer,
		ttl:           ttl,
		appURL:        strings.TrimRight(appURL, "/"),
		registration:  registration,
		now:           time.Now,
		dummyHash:     dummy,
	}, nil
}

// EmailEnabled reports whether a mail server is configured.
func (s *Service) EmailEnabled() bool {
	return s.mailer != nil
}

// RequiresVerifiedEmail reports whether accounts must verify their email
// before signing in, which needs a mail server to send the link.
func (s *Service) RequiresVerifiedEmail() bool {
	return s.EmailEnabled()
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmail(email string) error {
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return &domain.ValidationError{Field: "email", Message: "invalid email address", Code: "invalid_email"}
	}
	return nil
}

func validatePassword(field, password string) error {
	if len(password) < MinPasswordLength {
		return &domain.ValidationError{Field: field, Message: "must be at least 8 characters", Code: "password_too_short", Params: map[string]any{"min": 8}}
	}
	// bcrypt ignores anything past 72 bytes.
	if len(password) > 72 {
		return &domain.ValidationError{Field: field, Message: "must be at most 72 characters", Code: "password_too_long", Params: map[string]any{"max": 72}}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	// Drop a rune cut in half.
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
