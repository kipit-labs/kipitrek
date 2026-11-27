package hike

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/domain"
)

type fakeRepo struct {
	hikes []*domain.Hike
	tags  map[uuid.UUID][]uuid.UUID // hike id -> tagged users
}

func (f *fakeRepo) Create(_ context.Context, h *domain.Hike) error {
	f.hikes = append(f.hikes, h)
	return nil
}

func (f *fakeRepo) ListByUser(_ context.Context, userID uuid.UUID) ([]domain.Hike, error) {
	var out []domain.Hike
	for _, h := range f.hikes {
		if h.UserID == userID || slices.Contains(f.tags[h.ID], userID) {
			out = append(out, *h)
		}
	}
	return out, nil
}

func (f *fakeRepo) GetByID(_ context.Context, userID, id uuid.UUID) (*domain.Hike, error) {
	for _, h := range f.hikes {
		if h.UserID == userID && h.ID == id {
			return h, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeRepo) Find(_ context.Context, id uuid.UUID) (*domain.Hike, error) {
	for _, h := range f.hikes {
		if h.ID == id {
			return h, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (f *fakeRepo) Delete(_ context.Context, userID, id uuid.UUID) error {
	for i, h := range f.hikes {
		if h.UserID == userID && h.ID == id {
			f.hikes = append(f.hikes[:i], f.hikes[i+1:]...)
			return nil
		}
	}
	return domain.ErrNotFound
}

func (f *fakeRepo) DeleteMany(_ context.Context, userID uuid.UUID, ids []uuid.UUID) (int64, error) {
	var n int64
	for _, id := range ids {
		if f.Delete(context.Background(), userID, id) == nil {
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) Update(ctx context.Context, userID, id uuid.UUID, u domain.HikeUpdate) error {
	h, err := f.GetByID(ctx, userID, id)
	if err != nil {
		return err
	}
	if u.Name != nil {
		h.Name = *u.Name
	}
	if u.Notes != nil {
		h.Notes = *u.Notes
	}
	if u.Labels != nil {
		h.Labels = *u.Labels
	}
	if u.Planned != nil {
		h.Planned = *u.Planned
	}
	return nil
}

func (f *fakeRepo) ListLabels(_ context.Context, userID uuid.UUID) ([]string, error) {
	var out []string
	for _, h := range f.hikes {
		if h.UserID == userID {
			out = append(out, h.Labels...)
		}
	}
	return out, nil
}

func (f *fakeRepo) GetRawGPX(ctx context.Context, userID, id uuid.UUID) ([]byte, error) {
	h, err := f.GetByID(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	return h.RawGPX, nil
}

func (f *fakeRepo) ListTracks(context.Context, uuid.UUID, float64) ([]domain.HikeTrack, error) {
	return nil, nil
}

func (f *fakeRepo) ListParticipants(_ context.Context, id uuid.UUID) ([]domain.User, error) {
	var out []domain.User
	for _, u := range f.tags[id] {
		out = append(out, domain.User{ID: u})
	}
	return out, nil
}

func (f *fakeRepo) AddParticipant(_ context.Context, id, user uuid.UUID, _ time.Time) error {
	if f.tags == nil {
		f.tags = map[uuid.UUID][]uuid.UUID{}
	}
	if !slices.Contains(f.tags[id], user) {
		f.tags[id] = append(f.tags[id], user)
	}
	return nil
}

func (f *fakeRepo) RemoveParticipant(_ context.Context, id, user uuid.UUID) error {
	f.tags[id] = slices.DeleteFunc(f.tags[id], func(u uuid.UUID) bool { return u == user })
	return nil
}

func (f *fakeRepo) ListOutdated(_ context.Context, version, limit int) ([]domain.Hike, error) {
	var out []domain.Hike
	for _, h := range f.hikes {
		if h.DerivedVersion < version && len(out) < limit {
			out = append(out, domain.Hike{ID: h.ID, RawGPX: h.RawGPX})
		}
	}
	return out, nil
}

func (f *fakeRepo) ReplaceTrack(ctx context.Context, userID, id uuid.UUID, t *domain.Hike) error {
	h, err := f.GetByID(ctx, userID, id)
	if err != nil {
		return err
	}
	h.Planned = false
	h.DistanceM, h.StartedAt, h.DurationS = t.DistanceM, t.StartedAt, t.DurationS
	h.HikeDerived, h.Segments, h.Bounds, h.RawGPX = t.HikeDerived, t.Segments, t.Bounds, t.RawGPX
	return nil
}

func (f *fakeRepo) SaveDerived(_ context.Context, id uuid.UUID, d domain.HikeDerived, version int) error {
	h, err := f.Find(context.Background(), id)
	if err != nil {
		return err
	}
	h.HikeDerived, h.DerivedVersion = d, version
	return nil
}

func (f *fakeRepo) ListSimilar(_ context.Context, userID, hikeID uuid.UUID, _ float64) ([]domain.Hike, error) {
	var out []domain.Hike
	for _, h := range f.hikes {
		if h.ID != hikeID && (h.UserID == userID || slices.Contains(f.tags[h.ID], userID)) {
			out = append(out, *h)
		}
	}
	return out, nil
}

func (f *fakeRepo) ListFeed(_ context.Context, userID uuid.UUID, after *domain.FeedCursor, limit int) ([]domain.Hike, error) {
	var out []domain.Hike
	for _, h := range f.hikes {
		if h.UserID != userID && (after == nil || h.CreatedAt.Before(after.At)) && len(out) < limit {
			out = append(out, *h)
		}
	}
	return out, nil
}

func (f *fakeRepo) GetTrack(ctx context.Context, id uuid.UUID, _ float64) ([]domain.Segment, error) {
	h, err := f.Find(ctx, id)
	if err != nil {
		return nil, err
	}
	return h.Segments, nil
}

func (f *fakeRepo) ListTiles(_ context.Context, userID uuid.UUID) (map[uuid.UUID][]domain.Tile, error) {
	out := map[uuid.UUID][]domain.Tile{}
	for _, h := range f.hikes {
		if h.UserID == userID {
			out[h.ID] = h.Tiles
		}
	}
	return out, nil
}

// ownerOnly lets users see their own hikes, plus those of anyone in shared.
// Everyone in friends is friends with everyone else in it.
type ownerOnly struct{ shared, friends map[uuid.UUID]bool }

func (a ownerOnly) CanView(_ context.Context, viewer, owner uuid.UUID) (bool, error) {
	return viewer == owner || a.shared[owner], nil
}

func (a ownerOnly) AreFriends(_ context.Context, x, y uuid.UUID) (bool, error) {
	return a.friends[x] && a.friends[y], nil
}

type fakeParser struct {
	res     *domain.ParsedTrack
	err     error
	samples []domain.Sample
}

// Route marks the data it strips so tests can tell it was called.
func (p fakeParser) Route(data []byte) ([]byte, error) {
	return append([]byte("route:"), data...), p.err
}

func (p fakeParser) Parse([]byte) (*domain.ParsedTrack, error) { return p.res, p.err }

func (p fakeParser) Samples([]byte) ([]domain.Sample, error) { return p.samples, p.err }

func rename(name string) domain.HikeUpdate { return domain.HikeUpdate{Name: &name} }
