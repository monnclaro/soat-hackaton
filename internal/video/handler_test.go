package video

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

type apiFixture struct {
	mux   *http.ServeMux
	svc   *Service
	repo  *fakeRepo
	st    *fakeStorage
	token string
}

func newAPI(t *testing.T, maxBytes int64) apiFixture {
	t.Helper()
	tm, _ := security.NewTokenManager("0123456789abcdef0123456789abcdef", time.Hour)
	tok, _, _ := tm.Issue(ana)
	svc, repo, _, st := newSvc()
	mux := http.NewServeMux()
	NewHandler(svc, tm, discardLogger(), maxBytes).Register(mux)
	return apiFixture{mux: mux, svc: svc, repo: repo, st: st, token: tok}
}

func multipartBody(t *testing.T, field, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("description", "ignored")
	fw, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(content))
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func (f apiFixture) do(t *testing.T, req *http.Request, auth bool) *httptest.ResponseRecorder {
	t.Helper()
	if auth {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}
	rr := httptest.NewRecorder()
	f.mux.ServeHTTP(rr, req)
	return rr
}

func TestHTTPUploadListGetDownload(t *testing.T) {
	f := newAPI(t, 1<<20)

	body, ct := multipartBody(t, "video", "aula.mp4", "fake-video")
	req := httptest.NewRequest(http.MethodPost, "/api/videos", body)
	req.Header.Set("Content-Type", ct)
	rr := f.do(t, req, true)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("upload = %d %s", rr.Code, rr.Body)
	}
	var up Response
	_ = json.Unmarshal(rr.Body.Bytes(), &up)
	if up.Status != events.StatusPending || up.ID == "" || rr.Header().Get("Location") != "/api/videos/"+up.ID {
		t.Fatalf("upload response = %+v", up)
	}

	rr = f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos", nil), true)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), up.ID) {
		t.Fatalf("list = %d %s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "storage_key") || strings.Contains(rr.Body.String(), "user_email") {
		t.Fatal("internal fields must not leak")
	}

	rr = f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID, nil), true)
	if rr.Code != http.StatusOK {
		t.Fatalf("get = %d", rr.Code)
	}
	rr = f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos/unknown", nil), true)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("get unknown = %d", rr.Code)
	}

	rr = f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID+"/download", nil), true)
	if rr.Code != http.StatusConflict {
		t.Fatalf("download pending = %d", rr.Code)
	}

	zipKey := "outputs/x.zip"
	f.st.objects[zipKey] = []byte("PKzip")
	_ = f.svc.ApplyStatus(context.Background(), events.VideoStatusChanged{VideoID: up.ID, Status: events.StatusCompleted, ZipKey: zipKey, FrameCount: 3})

	rr = f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID, nil), true)
	if !strings.Contains(rr.Body.String(), `"download_url":"/api/videos/`+up.ID+`/download"`) {
		t.Fatalf("completed should expose download_url: %s", rr.Body)
	}

	rr = f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID+"/download", nil), true)
	if rr.Code != http.StatusOK || rr.Body.String() != "PKzip" {
		t.Fatalf("download = %d %q", rr.Code, rr.Body)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type = %s", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "aula_frames.zip") {
		t.Fatalf("content-disposition = %s", cd)
	}

	// ZIP removido do storage → 404
	delete(f.st.objects, zipKey)
	rr = f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID+"/download", nil), true)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("download missing zip = %d", rr.Code)
	}
}

func TestHTTPUploadValidation(t *testing.T) {
	f := newAPI(t, 1024)

	// sem token
	rr := f.do(t, httptest.NewRequest(http.MethodPost, "/api/videos", nil), false)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no auth = %d", rr.Code)
	}

	// não-multipart
	req := httptest.NewRequest(http.MethodPost, "/api/videos", strings.NewReader("raw"))
	req.Header.Set("Content-Type", "application/json")
	if rr := f.do(t, req, true); rr.Code != http.StatusBadRequest {
		t.Fatalf("non multipart = %d", rr.Code)
	}

	// campo errado
	body, ct := multipartBody(t, "file", "a.mp4", "x")
	req = httptest.NewRequest(http.MethodPost, "/api/videos", body)
	req.Header.Set("Content-Type", ct)
	if rr := f.do(t, req, true); rr.Code != http.StatusBadRequest {
		t.Fatalf("missing field = %d", rr.Code)
	}

	// extensão inválida
	body, ct = multipartBody(t, "video", "a.txt", "x")
	req = httptest.NewRequest(http.MethodPost, "/api/videos", body)
	req.Header.Set("Content-Type", ct)
	if rr := f.do(t, req, true); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad ext = %d", rr.Code)
	}

	// Content-Length acima do limite
	body, ct = multipartBody(t, "video", "a.mp4", strings.Repeat("x", 2000))
	req = httptest.NewRequest(http.MethodPost, "/api/videos", body)
	req.Header.Set("Content-Type", ct)
	if rr := f.do(t, req, true); rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large = %d", rr.Code)
	}

	// Corpo acima do limite sem Content-Length (chunked) → MaxBytesReader
	body, ct = multipartBody(t, "video", "a.mp4", strings.Repeat("x", 2000))
	req = httptest.NewRequest(http.MethodPost, "/api/videos", body)
	req.ContentLength = -1
	req.Header.Set("Content-Type", ct)
	if rr := f.do(t, req, true); rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large chunked = %d %s", rr.Code, rr.Body)
	}
}

func TestHTTPInternalErrors(t *testing.T) {
	f := newAPI(t, 1<<20)
	f.repo.getErr = errors.New("db down")
	if rr := f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos", nil), true); rr.Code != http.StatusInternalServerError {
		t.Fatalf("list infra error = %d", rr.Code)
	}
	if rr := f.do(t, httptest.NewRequest(http.MethodGet, "/api/videos/x", nil), true); rr.Code != http.StatusInternalServerError {
		t.Fatalf("get infra error = %d", rr.Code)
	}
}
