package handler

import (
	"archive/zip"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/delivery/http/dto"
	"github.com/kipit-labs/kipitrek/internal/delivery/http/middleware"
	"github.com/kipit-labs/kipitrek/internal/domain"
)

// GPX downloads the hike's original GPX file.
func (h *HikeHandler) GPX(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	hike, raw, err := h.svc.GPX(r.Context(), middleware.ViewerID(r.Context()), id)
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/gpx+xml")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": gpxFilename(hike.Name)}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(raw)
}

// gpxFilename turns a hike name into a safe download filename.
func gpxFilename(name string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if clean == "" {
		clean = "hike"
	}
	return clean + ".gpx"
}

// Export downloads a zip of the original GPX files of every hike the
// signed-in user owns. Hikes are read one at a time and streamed, so an
// error after the first file can only abort the download.
func (h *HikeHandler) Export(w http.ResponseWriter, r *http.Request) {
	h.export(w, r, nil)
}

// ExportSelected zips the GPX of the hikes in {ids} the user owns.
func (h *HikeHandler) ExportSelected(w http.ResponseWriter, r *http.Request) {
	var in dto.HikeIDs
	if !decodeJSON(w, r, &in) {
		return
	}
	ids, ok := parseIDs(w, in.IDs)
	if !ok {
		return
	}
	h.export(w, r, ids)
}

// export streams a zip of the user's hikes, only ids unless nil. Headers are
// sent with the first file, so errors before it still get a JSON answer.
func (h *HikeHandler) export(w http.ResponseWriter, r *http.Request, ids []uuid.UUID) {
	user := middleware.UserFrom(r.Context())
	name := "hikes-" + time.Now().UTC().Format(time.DateOnly) + ".zip"
	started := false
	start := func() {
		if !started {
			started = true
			w.Header().Set("Content-Type", "application/zip")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		}
	}

	zw := zip.NewWriter(w)
	names := exportNames{}
	err := h.svc.Export(r.Context(), user.ID, ids, func(hike *domain.Hike, raw []byte) error {
		start()
		modified := hike.CreatedAt
		if hike.StartedAt != nil {
			modified = *hike.StartedAt
		}
		f, err := zw.CreateHeader(&zip.FileHeader{Name: names.next(hike), Method: zip.Deflate, Modified: modified})
		if err != nil {
			return err
		}
		_, err = f.Write(raw)
		return err
	})
	if err != nil && !started {
		writeError(w, r, err)
		return
	}
	if err == nil {
		start()
		err = zw.Close()
	}
	if err != nil && r.Context().Err() == nil {
		slog.Error("export hikes", "user", user.ID, "err", err)
		// Headers are sent; dropping the connection keeps a truncated zip
		// from looking like a complete download.
		panic(http.ErrAbortHandler)
	}
}

// exportNames names files in an export after the hike's date and name,
// numbering repeats so no file overwrites another.
type exportNames map[string]bool

func (used exportNames) next(h *domain.Hike) string {
	base := strings.TrimSuffix(gpxFilename(h.Name), ".gpx")
	if h.StartedAt != nil {
		base = h.StartedAt.Format(time.DateOnly) + " " + base
	}
	name := base + ".gpx"
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s (%d).gpx", base, i)
	}
	used[name] = true
	return name
}
