package auth

import (
	"context"
	"errors"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

// RequestPasswordReset emails a reset link to the account, if there is one.
// Unknown emails succeed silently so the form does not reveal who signed up.
// Without a mail server it fails with ErrEmailDisabled: an admin hands out
// the link instead.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	if !s.EmailEnabled() {
		return domain.ErrEmailDisabled
	}
	u, err := s.users.GetByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	prev, err := s.resets.GetByUserID(ctx, u.ID)
	if err == nil && s.now().Before(prev.CreatedAt.Add(PasswordResetCooldown)) {
		return nil
	}
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return s.sendPasswordReset(ctx, u)
}

// issueReset stores a new password reset token for the user, replacing any
// previous one, and returns it.
func (s *Service) issueReset(ctx context.Context, userID uuid.UUID, ttl time.Duration) (string, error) {
	token, err := newToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	p := &domain.PasswordReset{TokenHash: hashToken(token), UserID: userID, ExpiresAt: now.Add(ttl), CreatedAt: now}
	if err := s.resets.Replace(ctx, p); err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) sendPasswordReset(ctx context.Context, u *domain.User) (err error) {
	token, err := s.issueReset(ctx, u.ID, PasswordResetTTL)
	if err != nil {
		return err
	}
	link := s.appURL + "/reset-password?token=" + url.QueryEscape(token)
	defer func() {
		// An undelivered link must not hold the cooldown.
		if err != nil {
			_ = s.resets.DeleteByUserID(context.WithoutCancel(ctx), u.ID)
		}
	}()
	subject, body := emailsIn(u.Language).passwordReset.render(link)
	return s.mailer.Send(ctx, u.Email, subject, body)
}

// ResetPassword consumes a reset token, sets the new password, signs out
// every session and signs the user in.
func (s *Service) ResetPassword(ctx context.Context, token, password string) (string, *domain.Session, error) {
	if token == "" {
		return "", nil, domain.ErrInvalidToken
	}
	// Checked first so a rejected password does not burn the link.
	if err := validatePassword("password", password); err != nil {
		return "", nil, err
	}
	p, err := s.resets.Consume(ctx, hashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil, domain.ErrInvalidToken
	}
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	if !now.Before(p.ExpiresAt) {
		return "", nil, domain.ErrInvalidToken
	}
	u, err := s.users.GetByID(ctx, p.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return "", nil, domain.ErrInvalidToken
	}
	if err != nil {
		return "", nil, err
	}
	if u.BannedAt != nil {
		return "", nil, &domain.BannedError{Reason: u.BanReason}
	}
	if err := s.setPassword(ctx, u.ID, password); err != nil {
		return "", nil, err
	}
	// Following the link, emailed or handed over by an admin, vouches for the
	// address, and accepts a pending invite.
	if u.EmailVerifiedAt == nil {
		if err := s.users.MarkEmailVerified(ctx, u.ID, now); err != nil {
			return "", nil, err
		}
	}
	if u.InvitedAt != nil {
		if err := s.users.AcceptInvite(ctx, u.ID); err != nil {
			return "", nil, err
		}
	}
	return s.startSession(ctx, u.ID)
}

// ChangePassword replaces the password of a signed-in user after checking the
// current one. Every session is signed out and a new one is returned.
func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, current, password string) (string, *domain.Session, error) {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	if err := s.hasher.Compare(u.PasswordHash, current); err != nil {
		return "", nil, &domain.ValidationError{Field: "currentPassword", Message: "current password is incorrect", Code: "wrong_password"}
	}
	if err := validatePassword("password", password); err != nil {
		return "", nil, err
	}
	if err := s.setPassword(ctx, u.ID, password); err != nil {
		return "", nil, err
	}
	return s.startSession(ctx, u.ID)
}

// setPassword stores a new password and revokes every session, so whoever
// knew the old one is signed out.
func (s *Service) setPassword(ctx context.Context, userID uuid.UUID, password string) error {
	hash, err := s.hasher.Hash(password)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePassword(ctx, userID, hash); err != nil {
		return err
	}
	if err := s.sessions.DeleteByUserID(ctx, userID); err != nil {
		return err
	}
	return s.resets.DeleteByUserID(ctx, userID)
}
