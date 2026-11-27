package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/delivery/http/dto"
	"github.com/kipit-labs/kipitrek/internal/delivery/http/middleware"
	"github.com/kipit-labs/kipitrek/internal/domain"
)

const maxFilesPerUpload = 50

type HikeService interface {
	Import(ctx context.Context, userID uuid.UUID, filename string, data []byte) (*domain.Hike, error)
	ImportPlanned(ctx context.Context, userID uuid.UUID, filename string, data []byte) (*domain.Hike, error)
	List(ctx context.Context, viewer, owner uuid.UUID) ([]domain.Hike, error)
	Get(ctx context.Context, viewer, id uuid.UUID) (*domain.Hike, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	Update(ctx context.Context, userID, id uuid.UUID, u domain.HikeUpdate) (*domain.Hike, error)
	MarkDone(ctx context.Context, userID, id uuid.UUID, data []byte) (*domain.Hike, error)
	Labels(ctx context.Context, userID uuid.UUID) ([]string, error)
	Tracks(ctx context.Context, viewer, owner uuid.UUID) ([]domain.HikeTrack, error)
	Tiles(ctx context.Context, viewer, owner uuid.UUID) (map[uuid.UUID][]domain.Tile, error)
	Similar(ctx context.Context, viewer, id uuid.UUID) ([]domain.Hike, error)
	Feed(ctx context.Context, userID uuid.UUID, after *domain.FeedCursor) ([]domain.Hike, *domain.FeedCursor, error)
	Card(ctx context.Context, viewer, id uuid.UUID) (*domain.Hike, []domain.Segment, error)
	Profile(ctx context.Context, viewer, id uuid.UUID) (*domain.Profile, error)
	GPX(ctx context.Context, viewer, id uuid.UUID) (*domain.Hike, []byte, error)
	Export(ctx context.Context, userID uuid.UUID, ids []uuid.UUID, fn func(h *domain.Hike, raw []byte) error) error
	DeleteMany(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (int64, error)
	Tag(ctx context.Context, owner, id, friend uuid.UUID) error
	Untag(ctx context.Context, viewer, id, participant uuid.UUID) error
}

// InteractionCounts sums up kudos and comments for the hikes a response shows.
type InteractionCounts interface {
	Counts(ctx context.Context, viewer uuid.UUID, hikeIDs []uuid.UUID) (map[uuid.UUID]domain.HikeInteractions, error)
}

// CardRenderer draws a hike's link preview image as PNG.
type CardRenderer func(h *domain.Hike, track []domain.Segment) ([]byte, error)

type HikeHandler struct {
	svc          HikeService
	interactions InteractionCounts
	renderCard   CardRenderer
	maxFileSize  int64
}

// NewHikeHandler builds the hike endpoints. interactions may be nil, leaving
// kudos and comment counts out of responses; renderCard may be nil when
// preview images are not served.
func NewHikeHandler(svc HikeService, interactions InteractionCounts, renderCard CardRenderer, maxFileSize int64) *HikeHandler {
	return &HikeHandler{svc: svc, interactions: interactions, renderCard: renderCard, maxFileSize: maxFileSize}
}

// withInteractions adds kudos and comment counts to hikes in a response.
func (h *HikeHandler) withInteractions(ctx context.Context, viewer uuid.UUID, hikes []dto.Hike) error {
	if h.interactions == nil || len(hikes) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(hikes))
	for i, hk := range hikes {
		ids[i] = uuid.MustParse(hk.ID)
	}
	counts, err := h.interactions.Counts(ctx, viewer, ids)
	if err != nil {
		return err
	}
	for i := range hikes {
		hikes[i].Interactions = dto.NewHikeInteractions(counts[ids[i]])
	}
	return nil
}

// List returns the signed-in user's hikes.
func (h *HikeHandler) List(w http.ResponseWriter, r *http.Request) {
	h.list(w, r, middleware.UserFrom(r.Context()).ID)
}

// UserList returns the hikes of the user in the URL, if the viewer may see them.
func (h *HikeHandler) UserList(w http.ResponseWriter, r *http.Request) {
	if owner, ok := parseID(w, r); ok {
		h.list(w, r, owner)
	}
}

func (h *HikeHandler) list(w http.ResponseWriter, r *http.Request, owner uuid.UUID) {
	hikes, err := h.svc.List(r.Context(), middleware.ViewerID(r.Context()), owner)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]dto.Hike, len(hikes))
	for i := range hikes {
		out[i] = dto.NewHike(&hikes[i])
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *HikeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	viewer := middleware.ViewerID(r.Context())
	hike, err := h.svc.Get(r.Context(), viewer, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := []dto.Hike{dto.NewHike(hike)}
	out[0].WeatherHours = hike.Weather
	if err := h.withInteractions(r.Context(), viewer, out); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, out[0])
}

func (h *HikeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var in dto.UpdateHike
	if !decodeJSON(w, r, &in) {
		return
	}
	if in.Name == nil && in.Notes == nil && in.Labels == nil {
		writeJSON(w, http.StatusBadRequest, dto.Error{Error: "nothing to update", Code: "invalid_body"})
		return
	}
	u := domain.HikeUpdate{Name: in.Name, Notes: in.Notes, Labels: in.Labels}
	hike, err := h.svc.Update(r.Context(), middleware.UserFrom(r.Context()).ID, id, u)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewHike(hike))
}

func (h *HikeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if err := h.svc.Delete(r.Context(), middleware.UserFrom(r.Context()).ID, id); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HikeHandler) Profile(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	p, err := h.svc.Profile(r.Context(), middleware.ViewerID(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewProfile(p))
}

// DeleteMany deletes the hikes in {ids} the user owns, skipping the others.
func (h *HikeHandler) DeleteMany(w http.ResponseWriter, r *http.Request) {
	var in dto.HikeIDs
	if !decodeJSON(w, r, &in) {
		return
	}
	ids, ok := parseIDs(w, in.IDs)
	if !ok {
		return
	}
	n, err := h.svc.DeleteMany(r.Context(), middleware.UserFrom(r.Context()).ID, ids)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.Deleted{Deleted: n})
}

func parseIDs(w http.ResponseWriter, raw []string) ([]uuid.UUID, bool) {
	ids := make([]uuid.UUID, len(raw))
	for i, s := range raw {
		id, err := uuid.Parse(s)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, dto.Error{Error: "invalid hike id", Field: "ids", Code: "invalid_body"})
			return nil, false
		}
		ids[i] = id
	}
	return ids, true
}

// Labels returns the labels the signed-in user has put on their hikes.
func (h *HikeHandler) Labels(w http.ResponseWriter, r *http.Request) {
	labels, err := h.svc.Labels(r.Context(), middleware.UserFrom(r.Context()).ID)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if labels == nil {
		labels = []string{}
	}
	writeJSON(w, http.StatusOK, labels)
}

// Feed returns a page of friends' recent hikes. `after` is the `next` cursor of
// the previous page.
func (h *HikeHandler) Feed(w http.ResponseWriter, r *http.Request) {
	var after *domain.FeedCursor
	if c := r.URL.Query().Get("after"); c != "" {
		cur, err := dto.ParseFeedCursor(c)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, dto.Error{Error: "invalid cursor", Field: "after", Code: "invalid_body"})
			return
		}
		after = cur
	}
	user := middleware.UserFrom(r.Context()).ID
	hikes, next, err := h.svc.Feed(r.Context(), user, after)
	if err != nil {
		writeError(w, r, err)
		return
	}
	feed := dto.NewFeed(hikes, next)
	if err := h.withInteractions(r.Context(), user, feed.Hikes); err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, feed)
}

// Card serves the hike's link preview image, for whoever may see the hike.
func (h *HikeHandler) Card(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	if h.renderCard == nil {
		writeJSON(w, http.StatusNotFound, dto.Error{Error: "not found", Code: "not_found"})
		return
	}
	hike, track, err := h.svc.Card(r.Context(), middleware.ViewerID(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	png, err := h.renderCard(hike, track)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	// Previews are fetched by link unfurlers; an hour keeps renames showing up soon.
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(png)
}

// Similar returns the signed-in user's other hikes along the same route.
func (h *HikeHandler) Similar(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	hikes, err := h.svc.Similar(r.Context(), middleware.UserFrom(r.Context()).ID, id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	out := make([]dto.Hike, len(hikes))
	for i := range hikes {
		out[i] = dto.NewHike(&hikes[i])
	}
	writeJSON(w, http.StatusOK, out)
}

// Tiles returns the zoom-14 tiles each of the signed-in user's hikes passes through.
func (h *HikeHandler) Tiles(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFrom(r.Context()).ID
	tiles, err := h.svc.Tiles(r.Context(), user, user)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewHikeTiles(tiles))
}

// Tracks returns the signed-in user's simplified hike geometries.
func (h *HikeHandler) Tracks(w http.ResponseWriter, r *http.Request) {
	h.tracks(w, r, middleware.UserFrom(r.Context()).ID)
}

// UserTracks returns the tracks of the user in the URL, if the viewer may see them.
func (h *HikeHandler) UserTracks(w http.ResponseWriter, r *http.Request) {
	if owner, ok := parseID(w, r); ok {
		h.tracks(w, r, owner)
	}
}

func (h *HikeHandler) tracks(w http.ResponseWriter, r *http.Request, owner uuid.UUID) {
	tracks, err := h.svc.Tracks(r.Context(), middleware.ViewerID(r.Context()), owner)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewFeatureCollection(tracks))
}

// Tag adds the friend in the URL to the hike.
func (h *HikeHandler) Tag(w http.ResponseWriter, r *http.Request) {
	h.participant(w, r, h.svc.Tag)
}

// Untag removes the participant in the URL from the hike.
func (h *HikeHandler) Untag(w http.ResponseWriter, r *http.Request) {
	h.participant(w, r, h.svc.Untag)
}

func (h *HikeHandler) participant(w http.ResponseWriter, r *http.Request, action func(ctx context.Context, viewer, id, user uuid.UUID) error) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	user, ok := parseParam(w, r, "userId")
	if !ok {
		return
	}
	if err := action(r.Context(), middleware.UserFrom(r.Context()).ID, id, user); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return parseParam(w, r, "id")
}

func parseParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		writeJSON(w, http.StatusNotFound, dto.Error{Error: "not found", Code: "not_found"})
		return uuid.Nil, false
	}
	return id, true
}
