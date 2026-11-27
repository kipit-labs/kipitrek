package handler

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/kipit-labs/kipitrek/internal/delivery/http/dto"
	"github.com/kipit-labs/kipitrek/internal/delivery/http/middleware"
	"github.com/kipit-labs/kipitrek/internal/domain"
)

// Upload imports every file in the multipart "files" field and reports per-file
// results. With ?planned=true the files are routes to do rather than hikes done.
func (h *HikeHandler) Upload(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFrom(r.Context())
	importFile := h.svc.Import
	if r.URL.Query().Get("planned") == "true" {
		importFile = h.svc.ImportPlanned
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize*maxFilesPerUpload)

	mr, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, dto.Error{Error: "expected multipart/form-data", Code: "invalid_body"})
		return
	}

	results := []dto.UploadResult{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, dto.Error{Error: "invalid multipart body", Code: "invalid_body"})
			return
		}
		if part.FormName() != "files" || part.FileName() == "" {
			part.Close()
			continue
		}
		if len(results) >= maxFilesPerUpload {
			part.Close()
			writeJSON(w, http.StatusBadRequest, dto.Error{Error: fmt.Sprintf("at most %d files per upload", maxFilesPerUpload), Code: "too_many_files", Params: map[string]any{"max": maxFilesPerUpload}})
			return
		}

		res := dto.UploadResult{Filename: part.FileName()}
		data, err := io.ReadAll(io.LimitReader(part, h.maxFileSize+1))
		part.Close()
		switch {
		case err != nil:
			writeJSON(w, http.StatusBadRequest, dto.Error{Error: "failed to read upload", Code: "upload_failed"})
			return
		case int64(len(data)) > h.maxFileSize:
			res.Error = fmt.Sprintf("file exceeds %d MB", h.maxFileSize>>20)
			res.Code = "file_too_large"
			res.Params = map[string]any{"max": h.maxFileSize >> 20}
		default:
			hike, err := importFile(r.Context(), user.ID, part.FileName(), data)
			if err != nil {
				if !errors.Is(err, domain.ErrInvalidGPX) {
					writeError(w, r, err)
					return
				}
				res.Error = "not a valid GPX file"
				res.Code = "invalid_gpx"
			} else {
				d := dto.NewHike(hike)
				res.Hike = &d
			}
		}
		results = append(results, res)
	}

	if len(results) == 0 {
		writeJSON(w, http.StatusBadRequest, dto.Error{Error: "no files provided", Code: "no_files"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// MarkDone marks a planned hike as walked. An optional multipart "file" holds
// the GPX recorded on the walk, which replaces the planned route.
func (h *HikeHandler) MarkDone(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	var data []byte
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct == "multipart/form-data" {
		r.Body = http.MaxBytesReader(w, r.Body, h.maxFileSize+1<<20)
		file, _, err := r.FormFile("file")
		switch {
		case errors.Is(err, http.ErrMissingFile):
		case err != nil:
			writeJSON(w, http.StatusBadRequest, dto.Error{Error: "invalid multipart body", Code: "invalid_body"})
			return
		default:
			data, err = io.ReadAll(io.LimitReader(file, h.maxFileSize+1))
			file.Close()
			if err != nil {
				writeJSON(w, http.StatusBadRequest, dto.Error{Error: "failed to read upload", Code: "upload_failed"})
				return
			}
			if int64(len(data)) > h.maxFileSize {
				writeJSON(w, http.StatusBadRequest, dto.Error{Error: fmt.Sprintf("file exceeds %d MB", h.maxFileSize>>20), Code: "file_too_large", Params: map[string]any{"max": h.maxFileSize >> 20}})
				return
			}
		}
	}
	hike, err := h.svc.MarkDone(r.Context(), middleware.UserFrom(r.Context()).ID, id, data)
	if errors.Is(err, domain.ErrInvalidGPX) {
		writeJSON(w, http.StatusBadRequest, dto.Error{Error: "not a valid GPX file", Code: "invalid_gpx"})
		return
	}
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, dto.NewHike(hike))
}
