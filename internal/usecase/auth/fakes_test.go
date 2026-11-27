package auth

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

type fakeUsers struct{ byID map[uuid.UUID]*domain.User }

func (f *fakeUsers) Create(_ context.Context, u *domain.User) error {
	for _, e := range f.byID {
		if e.Email == u.Email {
			return domain.ErrEmailTaken
		}
	}
	f.byID[u.ID] = u
	return nil
}

func (f *fakeUsers) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	if u, ok := f.byID[id]; ok {
		return u, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUsers) GetByEmail(_ context.Context, email string) (*domain.User, error) {
	for _, u := range f.byID {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeUsers) UpdatePassword(_ context.Context, id uuid.UUID, hash string) error {
	u, ok := f.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.PasswordHash = hash
	return nil
}

func (f *fakeUsers) Delete(_ context.Context, id uuid.UUID) error {
	if _, ok := f.byID[id]; !ok {
		return domain.ErrNotFound
	}
	delete(f.byID, id)
	return nil
}

func (f *fakeUsers) Count(context.Context) (int64, error) {
	return int64(len(f.byID)), nil
}

func (f *fakeUsers) CountAdmins(context.Context) (int64, error) {
	var n int64
	for _, u := range f.byID {
		if u.IsAdmin {
			n++
		}
	}
	return n, nil
}

func (f *fakeUsers) AcceptInvite(_ context.Context, id uuid.UUID) error {
	u, ok := f.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.InvitedAt = nil
	return nil
}

func (f *fakeUsers) MarkEmailVerified(_ context.Context, id uuid.UUID, at time.Time) error {
	u, ok := f.byID[id]
	if !ok {
		return domain.ErrNotFound
	}
	u.EmailVerifiedAt = &at
	return nil
}

type fakeSessions struct{ byHash map[string]*domain.Session }

func (f *fakeSessions) Create(_ context.Context, s *domain.Session) error {
	f.byHash[s.TokenHash] = s
	return nil
}

func (f *fakeSessions) GetByTokenHash(_ context.Context, h string) (*domain.Session, error) {
	if s, ok := f.byHash[h]; ok {
		return s, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeSessions) Delete(_ context.Context, h string) error {
	delete(f.byHash, h)
	return nil
}

func (f *fakeSessions) DeleteByUserID(_ context.Context, id uuid.UUID) error {
	for h, s := range f.byHash {
		if s.UserID == id {
			delete(f.byHash, h)
		}
	}
	return nil
}

func (f *fakeSessions) ListByUserID(_ context.Context, id uuid.UUID, now time.Time) ([]domain.Session, error) {
	var out []domain.Session
	for _, s := range f.byHash {
		if s.UserID == id && now.Before(s.ExpiresAt) {
			out = append(out, *s)
		}
	}
	return out, nil
}

func (f *fakeSessions) Touch(_ context.Context, h string, at time.Time, ip string) error {
	if s, ok := f.byHash[h]; ok {
		s.LastUsedAt, s.IP = at, ip
	}
	return nil
}

func (f *fakeSessions) DeleteByID(_ context.Context, userID, id uuid.UUID) error {
	for h, s := range f.byHash {
		if s.UserID == userID && s.ID == id {
			delete(f.byHash, h)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeSessions) DeleteOthers(_ context.Context, userID uuid.UUID, keep string) error {
	for h, s := range f.byHash {
		if s.UserID == userID && h != keep {
			delete(f.byHash, h)
		}
	}
	return nil
}

func (f *fakeSessions) DeleteExpired(_ context.Context, now time.Time) error {
	for h, s := range f.byHash {
		if !now.Before(s.ExpiresAt) {
			delete(f.byHash, h)
		}
	}
	return nil
}

type fakeVerifications struct {
	byUser map[uuid.UUID]*domain.EmailVerification
}

func (f *fakeVerifications) Replace(_ context.Context, v *domain.EmailVerification) error {
	f.byUser[v.UserID] = v
	return nil
}

func (f *fakeVerifications) GetByUserID(_ context.Context, id uuid.UUID) (*domain.EmailVerification, error) {
	if v, ok := f.byUser[id]; ok {
		return v, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeVerifications) DeleteByUserID(_ context.Context, id uuid.UUID) error {
	delete(f.byUser, id)
	return nil
}

func (f *fakeVerifications) Consume(_ context.Context, h string) (*domain.EmailVerification, error) {
	for id, v := range f.byUser {
		if v.TokenHash == h {
			delete(f.byUser, id)
			return v, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeVerifications) DeleteExpired(_ context.Context, now time.Time) error {
	for id, v := range f.byUser {
		if !now.Before(v.ExpiresAt) {
			delete(f.byUser, id)
		}
	}
	return nil
}

type fakeResets struct {
	byUser map[uuid.UUID]*domain.PasswordReset
}

func (f *fakeResets) Replace(_ context.Context, p *domain.PasswordReset) error {
	f.byUser[p.UserID] = p
	return nil
}

func (f *fakeResets) GetByUserID(_ context.Context, id uuid.UUID) (*domain.PasswordReset, error) {
	if p, ok := f.byUser[id]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}

func (f *fakeResets) DeleteByUserID(_ context.Context, id uuid.UUID) error {
	delete(f.byUser, id)
	return nil
}

func (f *fakeResets) Consume(_ context.Context, h string) (*domain.PasswordReset, error) {
	for id, p := range f.byUser {
		if p.TokenHash == h {
			delete(f.byUser, id)
			return p, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeResets) DeleteExpired(_ context.Context, now time.Time) error {
	for id, p := range f.byUser {
		if !now.Before(p.ExpiresAt) {
			delete(f.byUser, id)
		}
	}
	return nil
}

type sentMail struct{ to, subject, body string }

type fakeMailer struct {
	sent []sentMail
	err  error
}

func (f *fakeMailer) Send(_ context.Context, to, subject, body string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, sentMail{to, subject, body})
	return nil
}

// lastToken extracts the token from the link in the last email sent.
func (f *fakeMailer) lastToken(t *testing.T) string {
	t.Helper()
	if len(f.sent) == 0 {
		t.Fatal("no email sent")
	}
	m := linkRe.FindStringSubmatch(f.sent[len(f.sent)-1].body)
	if m == nil {
		t.Fatalf("no link in %q", f.sent[len(f.sent)-1].body)
	}
	token, err := url.QueryUnescape(m[1])
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// plainHasher avoids bcrypt cost in tests.
type plainHasher struct{}

func (plainHasher) Hash(p string) (string, error) { return "h:" + p, nil }

func (plainHasher) Compare(h, p string) error {
	if h != "h:"+p {
		return errors.New("mismatch")
	}
	return nil
}

type testEnv struct {
	svc           *Service
	users         *fakeUsers
	sessions      *fakeSessions
	verifications *fakeVerifications
	resets        *fakeResets
	mailer        *fakeMailer
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	e := &testEnv{
		users:         &fakeUsers{byID: map[uuid.UUID]*domain.User{}},
		sessions:      &fakeSessions{byHash: map[string]*domain.Session{}},
		verifications: &fakeVerifications{byUser: map[uuid.UUID]*domain.EmailVerification{}},
		resets:        &fakeResets{byUser: map[uuid.UUID]*domain.PasswordReset{}},
		mailer:        &fakeMailer{},
	}
	svc, err := NewService(e.users, e.sessions, e.verifications, e.resets, plainHasher{}, e.mailer, time.Hour, "https://app.test/", true)
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func newTestService(t *testing.T) (*Service, *fakeSessions) {
	t.Helper()
	e := newTestEnv(t)
	return e.svc, e.sessions
}

// registerVerified registers a user and follows their verification link.
func (e *testEnv) registerVerified(t *testing.T, email, password string) *domain.User {
	t.Helper()
	u, err := e.svc.Register(context.Background(), email, password, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.svc.VerifyEmail(context.Background(), e.mailer.lastToken(t)); err != nil {
		t.Fatal(err)
	}
	return u
}

// newTestEnvWithoutEmail is a service with no mail server configured.
func newTestEnvWithoutEmail(t *testing.T) *testEnv {
	t.Helper()
	e := newTestEnv(t)
	svc, err := NewService(e.users, e.sessions, e.verifications, e.resets, plainHasher{}, nil, time.Hour, "https://app.test/", true)
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e
}

func inviteToken(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil || u.Query().Get("invite") != "1" {
		t.Fatalf("bad invite link %q", link)
	}
	return u.Query().Get("token")
}
