package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/kipit-labs/kipitrek/internal/delivery/http/dto"
	"github.com/kipit-labs/kipitrek/internal/domain"
)

// importService accepts "<gpx>" files, rejects "bad" ones as invalid GPX and
// fails on "boom". It records which import ran and with what.
type importService struct {
	HikeService
	imported *[]string
}

func (s importService) record(kind, filename string, data []byte) (*domain.Hike, error) {
	switch string(data) {
	case "bad":
		return nil, domain.ErrInvalidGPX
	case "boom":
		return nil, errors.New("disk full")
	}
	*s.imported = append(*s.imported, kind+":"+filename)
	return &domain.Hike{ID: uuid.New(), Name: filename}, nil
}

func (s importService) Import(_ context.Context, _ uuid.UUID, filename string, data []byte) (*domain.Hike, error) {
	return s.record("done", filename, data)
}

func (s importService) ImportPlanned(_ context.Context, _ uuid.UUID, filename string, data []byte) (*domain.Hike, error) {
	return s.record("planned", filename, data)
}

func (s importService) MarkDone(_ context.Context, _, id uuid.UUID, data []byte) (*domain.Hike, error) {
	if string(data) == "bad" {
		return nil, domain.ErrInvalidGPX
	}
	*s.imported = append(*s.imported, "marked:"+string(data))
	return &domain.Hike{ID: id}, nil
}

type part struct{ field, filename, body string }

func multipartBody(t *testing.T, parts ...part) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range parts {
		var w interface{ Write([]byte) (int, error) }
		var err error
		if p.filename == "" {
			w, err = mw.CreateFormField(p.field)
		} else {
			w, err = mw.CreateFormFile(p.field, p.filename)
		}
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(p.body))
	}
	_ = mw.Close()
	return &buf, mw.FormDataContentType()
}

const maxSize = 1 << 10

func upload(t *testing.T, target string, parts ...part) (*[]string, int, []dto.UploadResult, string) {
	t.Helper()
	imported := &[]string{}
	body, ct := multipartBody(t, parts...)
	h := NewHikeHandler(importService{imported: imported}, nil, nil, maxSize)
	rec := call(h.Upload, http.MethodPost, "/hikes", target, body, &domain.User{ID: uuid.New()}, "Content-Type", ct)
	var out struct{ Results []dto.UploadResult }
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return imported, rec.Code, out.Results, rec.Body.String()
}

func TestUploadReportsEachFile(t *testing.T) {
	imported, status, results, body := upload(t, "/hikes",
		part{"files", "a.gpx", "<gpx>"},
		part{"note", "", "ignored field"},
		part{"files", "bad.gpx", "bad"},
		part{"files", "big.gpx", strings.Repeat("x", maxSize+1)},
		part{"files", "b.gpx", "<gpx>"},
	)
	if status != http.StatusOK {
		t.Fatalf("status %d: %s", status, body)
	}
	if got := strings.Join(*imported, ","); got != "done:a.gpx,done:b.gpx" {
		t.Errorf("imported %s", got)
	}
	want := []struct{ file, code string }{{"a.gpx", ""}, {"bad.gpx", "invalid_gpx"}, {"big.gpx", "file_too_large"}, {"b.gpx", ""}}
	if len(results) != len(want) {
		t.Fatalf("results %+v", results)
	}
	for i, w := range want {
		r := results[i]
		if r.Filename != w.file || r.Code != w.code || (w.code == "") != (r.Hike != nil) {
			t.Errorf("result %d = %+v, want %s %q", i, r, w.file, w.code)
		}
	}
}

func TestUploadPlanned(t *testing.T) {
	imported, status, _, _ := upload(t, "/hikes?planned=true", part{"files", "route.gpx", "<gpx>"})
	if status != http.StatusOK || strings.Join(*imported, ",") != "planned:route.gpx" {
		t.Errorf("status %d, imported %v", status, *imported)
	}
}

func TestUploadRejections(t *testing.T) {
	tooMany := make([]part, maxFilesPerUpload+1)
	for i := range tooMany {
		tooMany[i] = part{"files", "f.gpx", "<gpx>"}
	}
	cases := map[string]struct {
		parts  []part
		status int
		code   string
	}{
		"no files":       {[]part{{"note", "", "x"}}, http.StatusBadRequest, "no_files"},
		"too many files": {tooMany, http.StatusBadRequest, "too_many_files"},
		"server failure": {[]part{{"files", "a.gpx", "boom"}}, http.StatusInternalServerError, "internal"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, status, _, body := upload(t, "/hikes", c.parts...)
			var e dto.Error
			_ = json.Unmarshal([]byte(body), &e)
			if status != c.status || e.Code != c.code {
				t.Errorf("status %d code %q, want %d %q", status, e.Code, c.status, c.code)
			}
		})
	}

	h := NewHikeHandler(importService{imported: &[]string{}}, nil, nil, maxSize)
	rec := call(h.Upload, http.MethodPost, "/hikes", "/hikes", jsonBody(`{}`), &domain.User{ID: uuid.New()}, "Content-Type", "application/json")
	wantError(t, rec, http.StatusBadRequest, "invalid_body")
}

func TestMarkDone(t *testing.T) {
	id := uuid.New()
	target := "/hikes/" + id.String() + "/done"
	u := &domain.User{ID: uuid.New()}

	imported := &[]string{}
	h := NewHikeHandler(importService{imported: imported}, nil, nil, maxSize)
	if rec := call(h.MarkDone, http.MethodPost, "/hikes/{id}/done", target, nil, u); rec.Code != http.StatusOK {
		t.Fatalf("without a file: status %d", rec.Code)
	}

	body, ct := multipartBody(t, part{"file", "walk.gpx", "<gpx>"})
	if rec := call(h.MarkDone, http.MethodPost, "/hikes/{id}/done", target, body, u, "Content-Type", ct); rec.Code != http.StatusOK {
		t.Fatalf("with a file: status %d", rec.Code)
	}
	if got := strings.Join(*imported, ","); got != "marked:,marked:<gpx>" {
		t.Errorf("marked %s", got)
	}

	for name, c := range map[string]struct{ body, code string }{
		"invalid gpx": {"bad", "invalid_gpx"},
		"too large":   {strings.Repeat("x", maxSize+1), "file_too_large"},
	} {
		body, ct := multipartBody(t, part{"file", "walk.gpx", c.body})
		rec := call(h.MarkDone, http.MethodPost, "/hikes/{id}/done", target, body, u, "Content-Type", ct)
		t.Run(name, func(t *testing.T) { wantError(t, rec, http.StatusBadRequest, c.code) })
	}
}

func TestHikeUpdateNeedsAField(t *testing.T) {
	h := NewHikeHandler(importService{}, nil, nil, maxSize)
	rec := call(h.Update, http.MethodPatch, "/hikes/{id}", "/hikes/"+uuid.NewString(), jsonBody(`{}`), &domain.User{ID: uuid.New()})
	wantError(t, rec, http.StatusBadRequest, "invalid_body")
}

func TestDeleteManyRejectsBadIDs(t *testing.T) {
	h := NewHikeHandler(importService{}, nil, nil, maxSize)
	rec := call(h.DeleteMany, http.MethodPost, "/", "/", jsonBody(`{"ids":["`+uuid.NewString()+`","nope"]}`), &domain.User{ID: uuid.New()})
	if e := wantError(t, rec, http.StatusBadRequest, "invalid_body"); e.Field != "ids" {
		t.Errorf("field %q", e.Field)
	}
}
