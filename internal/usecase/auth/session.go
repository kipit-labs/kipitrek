package auth

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

// Login verifies credentials and returns a new opaque session token.
func (s *Service) Login(ctx context.Context, email, password string) (string, *domain.Session, error) {
	u, err := s.users.GetByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, domain.ErrNotFound) {
		_ = s.hasher.Compare(s.dummyHash, password)
		return "", nil, domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", nil, err
	}
	if err := s.hasher.Compare(u.PasswordHash, password); err != nil {
		return "", nil, domain.ErrInvalidCredentials
	}
	if u.BannedAt != nil {
		return "", nil, &domain.BannedError{Reason: u.BanReason}
	}
	if u.EmailVerifiedAt == nil && s.RequiresVerifiedEmail() {
		if err := s.ensureVerificationSent(ctx, u); err != nil {
			slog.Error("send verification email", "user", u.ID, "err", err)
		}
		return "", nil, domain.ErrEmailNotVerified
	}
	return s.startSession(ctx, u.ID)
}

func (s *Service) startSession(ctx context.Context, userID uuid.UUID) (string, *domain.Session, error) {
	token, err := newToken()
	if err != nil {
		return "", nil, err
	}
	now := s.now()
	client := domain.ClientFrom(ctx)
	sess := &domain.Session{
		ID:         uuid.New(),
		TokenHash:  hashToken(token),
		UserID:     userID,
		UserAgent:  truncate(client.UserAgent, maxUserAgentLength),
		IP:         client.IP,
		ExpiresAt:  now.Add(s.ttl),
		LastUsedAt: now,
		CreatedAt:  now,
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return "", nil, err
	}
	return token, sess, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.sessions.Delete(ctx, hashToken(token))
}

// Authenticate resolves a session token to its user.
func (s *Service) Authenticate(ctx context.Context, token string) (*domain.User, error) {
	if token == "" {
		return nil, domain.ErrUnauthorized
	}
	sess, err := s.sessions.GetByTokenHash(ctx, hashToken(token))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	now := s.now()
	if !now.Before(sess.ExpiresAt) {
		_ = s.sessions.Delete(ctx, sess.TokenHash)
		return nil, domain.ErrUnauthorized
	}
	if now.Sub(sess.LastUsedAt) >= sessionTouchInterval {
		// Only informs the sessions list; not worth failing the request.
		if err := s.sessions.Touch(ctx, sess.TokenHash, now, domain.ClientFrom(ctx).IP); err != nil {
			slog.Warn("touch session", "err", err)
		}
	}
	u, err := s.users.GetByID(ctx, sess.UserID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	// Banning revokes sessions; this covers a ban racing a sign-in.
	if u.BannedAt != nil {
		return nil, domain.ErrUnauthorized
	}
	return u, nil
}

// Sessions lists the user's live sessions, most recently used first, with
// the ID of the one token belongs to (uuid.Nil if none).
func (s *Service) Sessions(ctx context.Context, userID uuid.UUID, token string) ([]domain.Session, uuid.UUID, error) {
	list, err := s.sessions.ListByUserID(ctx, userID, s.now())
	if err != nil {
		return nil, uuid.Nil, err
	}
	current := uuid.Nil
	hash := hashToken(token)
	for _, sess := range list {
		if sess.TokenHash == hash {
			current = sess.ID
		}
	}
	return list, current, nil
}

// RevokeSession signs one of the user's other devices out. The session token
// belongs to is refused: signing out does that.
func (s *Service) RevokeSession(ctx context.Context, userID, id uuid.UUID, token string) error {
	if sess, err := s.sessions.GetByTokenHash(ctx, hashToken(token)); err == nil && sess.ID == id {
		return &domain.ValidationError{Field: "session", Message: "this is your current session; sign out instead", Code: "current_session"}
	}
	return s.sessions.DeleteByID(ctx, userID, id)
}

// RevokeOtherSessions signs the user out everywhere but the session of token.
func (s *Service) RevokeOtherSessions(ctx context.Context, userID uuid.UUID, token string) error {
	return s.sessions.DeleteOthers(ctx, userID, hashToken(token))
}

// PurgeExpired deletes expired sessions, verification and reset links.
func (s *Service) PurgeExpired(ctx context.Context) error {
	now := s.now()
	return errors.Join(
		s.sessions.DeleteExpired(ctx, now),
		s.verifications.DeleteExpired(ctx, now),
		s.resets.DeleteExpired(ctx, now),
	)
}
