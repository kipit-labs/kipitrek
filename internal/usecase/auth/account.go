package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/url"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

// RegistrationEnabled reports whether REGISTRATION_ENABLED lets anyone sign up.
func (s *Service) RegistrationEnabled() bool {
	return s.registration
}

// RegistrationOpen reports whether the sign-up form accepts new accounts:
// when registration is enabled, or to create the first account.
func (s *Service) RegistrationOpen(ctx context.Context) (bool, error) {
	if s.registration {
		return true, nil
	}
	n, err := s.users.Count(ctx)
	return n == 0, err
}

// Register creates an unverified account. With a mail server, it emails a
// verification link and the account cannot sign in until the link is
// followed; without one, it can sign in right away. The first account becomes
// the admin.
func (s *Service) Register(ctx context.Context, email, password string, lang domain.Language) (*domain.User, error) {
	email = normalizeEmail(email)
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if err := validatePassword("password", password); err != nil {
		return nil, err
	}
	n, err := s.users.Count(ctx)
	if err != nil {
		return nil, err
	}
	if n > 0 && !s.registration {
		return nil, domain.ErrRegistrationClosed
	}
	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return nil, domain.ErrEmailTaken
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}

	hash, err := s.hasher.Hash(password)
	if err != nil {
		return nil, err
	}
	if !lang.Valid() {
		lang = ""
	}
	u := &domain.User{ID: uuid.New(), Email: email, PasswordHash: hash, Visibility: domain.VisibilityPrivate, Language: lang, IsAdmin: n == 0, CreatedAt: s.now()}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	if !s.EmailEnabled() {
		return u, nil
	}
	// The account exists either way; the next sign-in attempt retries.
	if err := s.sendVerification(ctx, u); err != nil {
		slog.Error("send verification email", "user", u.ID, "err", err)
	}
	return u, nil
}

// Invite creates an account for email on behalf of an admin, whatever the
// registration setting, and returns a link where its owner chooses a
// password. Following the link also verifies the email. The link is emailed
// when send is set and a mail server is configured; a failed delivery is
// logged and reported by emailed, and the link still works.
func (s *Service) Invite(ctx context.Context, email, name string, lang domain.Language, send bool) (u *domain.User, link string, emailed bool, err error) {
	email = normalizeEmail(email)
	if err := validateEmail(email); err != nil {
		return nil, "", false, err
	}
	if _, err := s.users.GetByEmail(ctx, email); err == nil {
		return nil, "", false, domain.ErrEmailTaken
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, "", false, err
	}
	// Nobody knows this password, so the account is unusable until the link
	// is followed.
	secret, err := newToken()
	if err != nil {
		return nil, "", false, err
	}
	hash, err := s.hasher.Hash(secret)
	if err != nil {
		return nil, "", false, err
	}
	now := s.now()
	if !lang.Valid() {
		lang = ""
	}
	u = &domain.User{ID: uuid.New(), Email: email, Name: name, PasswordHash: hash, Visibility: domain.VisibilityPrivate, Language: lang, InvitedAt: &now, CreatedAt: now}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, "", false, err
	}
	link, emailed, err = s.sendPasswordLink(ctx, u, send)
	return u, link, emailed, err
}

// PasswordLink issues a new link where a user chooses a password, for an
// admin to hand over: the invite link again while an invite is pending,
// otherwise a password reset link, which is how people who forgot their
// password get back in without email. It is emailed when send is set and a
// mail server is configured.
func (s *Service) PasswordLink(ctx context.Context, userID uuid.UUID, send bool) (link string, emailed bool, err error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", false, err
	}
	if u.BannedAt != nil {
		return "", false, &domain.ValidationError{Field: "user", Message: "lift their ban first", Code: "banned_user"}
	}
	return s.sendPasswordLink(ctx, u, send)
}

func (s *Service) sendPasswordLink(ctx context.Context, u *domain.User, send bool) (link string, emailed bool, err error) {
	invite := u.InvitedAt != nil
	ttl := AdminResetTTL
	if invite {
		ttl = InviteTTL
	}
	token, err := s.issueReset(ctx, u.ID, ttl)
	if err != nil {
		return "", false, err
	}
	link = s.appURL + "/reset-password?token=" + url.QueryEscape(token)
	if invite {
		link = s.appURL + "/reset-password?invite=1&token=" + url.QueryEscape(token)
	}
	if !send || !s.EmailEnabled() {
		return link, false, nil
	}
	subject, body := emailsIn(u.Language).adminReset.render(link)
	if invite {
		subject, body = emailsIn(u.Language).invite.render(link)
	}
	if err := s.mailer.Send(ctx, u.Email, subject, body); err != nil {
		slog.Error("send password link", "user", u.ID, "err", err)
		return link, false, nil
	}
	return link, true, nil
}

// VerifyEmail consumes a verification token, marks the email verified and
// signs the user in.
func (s *Service) VerifyEmail(ctx context.Context, token string) (string, *domain.Session, error) {
	if token == "" {
		return "", nil, domain.ErrInvalidToken
	}
	v, err := s.verifications.Consume(ctx, hashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil, domain.ErrInvalidToken
	}
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	if !now.Before(v.ExpiresAt) {
		return "", nil, domain.ErrInvalidToken
	}
	u, err := s.users.GetByID(ctx, v.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil, domain.ErrInvalidToken
	}
	if err != nil {
		return "", nil, err
	}
	if err := s.users.MarkEmailVerified(ctx, u.ID, now); err != nil {
		return "", nil, err
	}
	if u.BannedAt != nil {
		return "", nil, &domain.BannedError{Reason: u.BanReason}
	}
	return s.startSession(ctx, u.ID)
}

// ensureVerificationSent emails a verification link unless a valid one was
// already sent, so repeated sign-in attempts cannot spam the inbox.
func (s *Service) ensureVerificationSent(ctx context.Context, u *domain.User) error {
	prev, err := s.verifications.GetByUserID(ctx, u.ID)
	if err == nil && s.now().Before(prev.ExpiresAt) {
		return nil
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return s.sendVerification(ctx, u)
}

func (s *Service) sendVerification(ctx context.Context, u *domain.User) (err error) {
	token, err := newToken()
	if err != nil {
		return err
	}
	now := s.now()
	v := &domain.EmailVerification{TokenHash: hashToken(token), UserID: u.ID, ExpiresAt: now.Add(VerificationTTL), CreatedAt: now}
	if err := s.verifications.Replace(ctx, v); err != nil {
		return err
	}
	link := s.appURL + "/verify?token=" + url.QueryEscape(token)
	defer func() {
		// An undelivered link must not block the next attempt until it expires.
		if err != nil {
			_ = s.verifications.DeleteByUserID(context.WithoutCancel(ctx), u.ID)
		}
	}()
	subject, body := emailsIn(u.Language).verification.render(link)
	return s.mailer.Send(ctx, u.Email, subject, body)
}

// DeleteAccount permanently removes a signed-in user and everything tied to
// them, after checking their password.
func (s *Service) DeleteAccount(ctx context.Context, userID uuid.UUID, password string) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.hasher.Compare(u.PasswordHash, password); err != nil {
		return &domain.ValidationError{Field: "password", Message: "password is incorrect", Code: "wrong_password"}
	}
	if u.IsAdmin {
		if err := s.ensureOtherAdmin(ctx); err != nil {
			return err
		}
	}
	return s.users.Delete(ctx, u.ID)
}

// ensureOtherAdmin refuses to remove the last admin while other accounts
// remain, so the instance always has someone to manage it.
func (s *Service) ensureOtherAdmin(ctx context.Context) error {
	admins, err := s.users.CountAdmins(ctx)
	if err != nil {
		return err
	}
	users, err := s.users.Count(ctx)
	if err != nil {
		return err
	}
	if admins <= 1 && users > 1 {
		return &domain.ValidationError{Field: "account", Message: "you are the only admin; make someone else an admin first", Code: "only_admin"}
	}
	return nil
}
