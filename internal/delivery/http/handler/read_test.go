package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/delivery/http/dto"
	"github.com/kipit-labs/kipitrek/internal/domain"
)

// readService serves one hike to its owner and to nobody else.
type readService struct {
	HikeService
	hike   domain.Hike
	viewer *uuid.UUID
}

func (s readService) see(viewer, id uuid.UUID) error {
	if s.viewer != nil {
		*s.viewer = viewer
	}
	if id != s.hike.ID || viewer != s.hike.UserID {
		return domain.ErrNotFound
	}
	return nil
}

func (s readService) Get(_ context.Context, viewer, id uuid.UUID) (*domain.Hike, error) {
	if err := s.see(viewer, id); err != nil {
		return nil, err
	}
	return &s.hike, nil
}

func (s readService) GPX(_ context.Context, viewer, id uuid.UUID) (*domain.Hike, []byte, error) {
	if err := s.see(viewer, id); err != nil {
		return nil, nil, err
	}
	return &s.hike, []byte("<gpx/>"), nil
}

func (s readService) Card(_ context.Context, viewer, id uuid.UUID) (*domain.Hike, []domain.Segment, error) {
	if err := s.see(viewer, id); err != nil {
		return nil, nil, err
	}
	return &s.hike, nil, nil
}

func (s readService) Labels(context.Context, uuid.UUID) ([]string, error) { return nil, nil }

func (s readService) Feed(_ context.Context, _ uuid.UUID, after *domain.FeedCursor) ([]domain.Hike, *domain.FeedCursor, error) {
	if after == nil {
		return []domain.Hike{s.hike}, &domain.FeedCursor{At: time.Now(), ID: s.hike.ID}, nil
	}
	return nil, nil, nil
}

type counts map[uuid.UUID]domain.HikeInteractions

func (c counts) Counts(context.Context, uuid.UUID, []uuid.UUID) (map[uuid.UUID]domain.HikeInteractions, error) {
	return c, nil
}

func newReadService() readService {
	return readService{hike: domain.Hike{ID: uuid.New(), UserID: uuid.New(), Name: `Col "du" Lac/2`}}
}

func TestGetAddsInteractions(t *testing.T) {
	s := newReadService()
	h := NewHikeHandler(s, counts{s.hike.ID: {Kudos: 3, Comments: 1, Kudoed: true}}, nil, 1)
	rec := call(h.Get, http.MethodGet, "/hikes/{id}", "/hikes/"+s.hike.ID.String(), nil, &domain.User{ID: s.hike.UserID})
	var out dto.Hike
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if out.Interactions == nil || *out.Interactions != (dto.HikeInteractions{Kudos: 3, Comments: 1, Kudoed: true}) {
		t.Errorf("interactions %+v", out.Interactions)
	}
}

// Signed-out viewers reach the service as uuid.Nil, which it treats as anonymous.
func TestAnonymousViewerIsNil(t *testing.T) {
	s := newReadService()
	viewer := uuid.New()
	s.viewer = &viewer
	rec := call(NewHikeHandler(s, nil, nil, 1).Get, http.MethodGet, "/hikes/{id}", "/hikes/"+s.hike.ID.String(), nil, nil)
	wantError(t, rec, http.StatusNotFound, "not_found")
	if viewer != uuid.Nil {
		t.Errorf("viewer %v, want uuid.Nil", viewer)
	}
}

func TestGPXDownload(t *testing.T) {
	s := newReadService()
	h := NewHikeHandler(s, nil, nil, 1)
	rec := call(h.GPX, http.MethodGet, "/hikes/{id}/gpx", "/hikes/"+s.hike.ID.String()+"/gpx", nil, &domain.User{ID: s.hike.UserID})
	if rec.Code != http.StatusOK || rec.Body.String() != "<gpx/>" {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	for k, want := range map[string]string{
		"Content-Type":           "application/gpx+xml",
		"Content-Disposition":    `attachment; filename="Col _du_ Lac_2.gpx"`,
		"X-Content-Type-Options": "nosniff",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	rec = call(h.GPX, http.MethodGet, "/hikes/{id}/gpx", "/hikes/"+s.hike.ID.String()+"/gpx", nil, &domain.User{ID: uuid.New()})
	wantError(t, rec, http.StatusNotFound, "not_found")
}

func TestCard(t *testing.T) {
	s := newReadService()
	target := "/hikes/" + s.hike.ID.String() + "/card.png"
	owner := &domain.User{ID: s.hike.UserID}

	rec := call(NewHikeHandler(s, nil, nil, 1).Card, http.MethodGet, "/hikes/{id}/card.png", target, nil, owner)
	wantError(t, rec, http.StatusNotFound, "not_found")

	render := func(*domain.Hike, []domain.Segment) ([]byte, error) { return []byte("png"), nil }
	rec = call(NewHikeHandler(s, nil, render, 1).Card, http.MethodGet, "/hikes/{id}/card.png", target, nil, owner)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/png" || rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Errorf("status %d, headers %v", rec.Code, rec.Header())
	}

	broken := func(*domain.Hike, []domain.Segment) ([]byte, error) { return nil, errors.New("font missing") }
	rec = call(NewHikeHandler(s, nil, broken, 1).Card, http.MethodGet, "/hikes/{id}/card.png", target, nil, owner)
	wantError(t, rec, http.StatusInternalServerError, "internal")
}

func TestFeedPaging(t *testing.T) {
	s := newReadService()
	h := NewHikeHandler(s, counts{}, nil, 1)
	u := &domain.User{ID: uuid.New()}

	rec := call(h.Feed, http.MethodGet, "/feed", "/feed", nil, u)
	var page dto.Feed
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Hikes) != 1 || page.Next == nil {
		t.Fatalf("first page %s", rec.Body)
	}
	rec = call(h.Feed, http.MethodGet, "/feed", "/feed?after="+*page.Next, nil, u)
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil || len(page.Hikes) != 0 || page.Next != nil {
		t.Fatalf("last page %s", rec.Body)
	}

	rec = call(h.Feed, http.MethodGet, "/feed", "/feed?after=yesterday", nil, u)
	if e := wantError(t, rec, http.StatusBadRequest, "invalid_body"); e.Field != "after" {
		t.Errorf("field %q", e.Field)
	}
}

// The web app maps over labels, so none is [] rather than null.
func TestLabelsNeverNull(t *testing.T) {
	rec := call(NewHikeHandler(newReadService(), nil, nil, 1).Labels, http.MethodGet, "/", "/", nil, &domain.User{ID: uuid.New()})
	if rec.Body.String() != "[]\n" {
		t.Errorf("body %q", rec.Body)
	}
}

// A failure after the first file is streamed must cut the connection, so a
// truncated zip never looks like a complete download.
func TestExportAbortsMidStream(t *testing.T) {
	svc := exportService{hikes: []domain.Hike{{Name: "a"}}, failAfter: true}
	defer func() {
		if r := recover(); !errors.Is(r.(error), http.ErrAbortHandler) {
			t.Errorf("recovered %v, want http.ErrAbortHandler", r)
		}
	}()
	call(NewHikeHandler(svc, nil, nil, 1).Export, http.MethodGet, "/", "/", nil, &domain.User{ID: uuid.New()})
	t.Error("export finished without aborting")
}

func TestTagNeedsAValidUser(t *testing.T) {
	rec := call(NewHikeHandler(newReadService(), nil, nil, 1).Tag, http.MethodPut, "/hikes/{id}/participants/{userId}",
		"/hikes/"+uuid.NewString()+"/participants/nope", nil, &domain.User{ID: uuid.New()})
	wantError(t, rec, http.StatusNotFound, "not_found")
}

type commentService struct{ InteractionService }

func (commentService) Comment(_ context.Context, _, _ uuid.UUID, body string) (*domain.Comment, error) {
	return &domain.Comment{ID: uuid.New(), Body: body}, nil
}

func TestPostCommentNamesTheAuthor(t *testing.T) {
	u := &domain.User{ID: uuid.New(), Name: "Ana"}
	rec := call(NewInteractionHandler(commentService{}).PostComment, http.MethodPost, "/hikes/{id}/comments",
		"/hikes/"+uuid.NewString()+"/comments", jsonBody(`{"body":"nice"}`), u)
	var c dto.Comment
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if c.Body != "nice" || c.Author == nil || c.Author.Name != "Ana" {
		t.Errorf("comment %+v", c)
	}
}
