package httpapi

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

	"github.com/monnclaro/soat-hackaton/internal/platform/security"
	"github.com/monnclaro/soat-hackaton/internal/video/domain"
	"github.com/monnclaro/soat-hackaton/internal/video/usecase"
	"github.com/monnclaro/soat-hackaton/internal/video/videotest"
)

type fixture struct {
	mux    *http.ServeMux
	videos *usecase.Videos
	repo   *videotest.Repo
	st     *videotest.Storage
	token  string
}

func newFixture(t *testing.T, maxBytes int64) fixture {
	t.Helper()
	tm, _ := security.NewTokenManager("0123456789abcdef0123456789abcdef", time.Hour)
	tok, _, _ := tm.Issue(security.Principal{UserID: "u1", Email: "ana@x.com", Name: "Ana"})
	repo, st := videotest.NewRepo(), videotest.NewStorage()
	videos := usecase.NewVideos(repo, videotest.NewCache(), st, videotest.DiscardLogger())
	mux := http.NewServeMux()
	NewHandler(videos, tm, videotest.DiscardLogger(), maxBytes).Register(mux)
	return fixture{mux: mux, videos: videos, repo: repo, st: st, token: tok}
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

func (f fixture) do(req *http.Request, auth bool) *httptest.ResponseRecorder {
	if auth {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}
	rr := httptest.NewRecorder()
	f.mux.ServeHTTP(rr, req)
	return rr
}

func TestUploadListGetDownload(t *testing.T) {
	f := newFixture(t, 1<<20)

	body, ct := multipartBody(t, "video", "aula.mp4", "fake-video")
	req := httptest.NewRequest(http.MethodPost, "/api/videos", body)
	req.Header.Set("Content-Type", ct)
	rr := f.do(req, true)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("upload = %d %s", rr.Code, rr.Body)
	}
	var up VideoResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &up)
	if up.Status != string(domain.StatusPending) || up.ID == "" || rr.Header().Get("Location") != "/api/videos/"+up.ID {
		t.Fatalf("upload response = %+v", up)
	}
	if got := f.repo.Video(up.ID); got.UserEmail != "ana@x.com" {
		t.Fatalf("owner e-mail must come from the token: %+v", got)
	}

	rr = f.do(httptest.NewRequest(http.MethodGet, "/api/videos", nil), true)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), up.ID) {
		t.Fatalf("list = %d %s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "storage_key") || strings.Contains(rr.Body.String(), "user_email") {
		t.Fatal("internal fields must not leak")
	}

	if rr := f.do(httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID, nil), true); rr.Code != http.StatusOK {
		t.Fatalf("get = %d", rr.Code)
	}
	if rr := f.do(httptest.NewRequest(http.MethodGet, "/api/videos/unknown", nil), true); rr.Code != http.StatusNotFound {
		t.Fatalf("get unknown = %d", rr.Code)
	}
	if rr := f.do(httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID+"/download", nil), true); rr.Code != http.StatusConflict {
		t.Fatalf("download pending = %d", rr.Code)
	}

	zipKey := "outputs/x.zip"
	f.st.Objects[zipKey] = []byte("PKzip")
	_, _ = f.videos.ApplyStatus(context.Background(), up.ID, domain.StatusChange{Status: domain.StatusCompleted, ZipKey: zipKey, FrameCount: 3})

	rr = f.do(httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID, nil), true)
	if !strings.Contains(rr.Body.String(), `"download_url":"/api/videos/`+up.ID+`/download"`) {
		t.Fatalf("completed must expose download_url: %s", rr.Body)
	}

	rr = f.do(httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID+"/download", nil), true)
	if rr.Code != http.StatusOK || rr.Body.String() != "PKzip" {
		t.Fatalf("download = %d %q", rr.Code, rr.Body)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("content-type = %s", ct)
	}
	if cd := rr.Header().Get("Content-Disposition"); !strings.Contains(cd, "aula_frames.zip") {
		t.Fatalf("content-disposition = %s", cd)
	}

	delete(f.st.Objects, zipKey)
	if rr := f.do(httptest.NewRequest(http.MethodGet, "/api/videos/"+up.ID+"/download", nil), true); rr.Code != http.StatusNotFound {
		t.Fatalf("download missing zip = %d", rr.Code)
	}
}

func TestUploadValidation(t *testing.T) {
	f := newFixture(t, 1024)

	if rr := f.do(httptest.NewRequest(http.MethodPost, "/api/videos", nil), false); rr.Code != http.StatusUnauthorized {
		t.Fatalf("no auth = %d", rr.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/videos", strings.NewReader("raw"))
	req.Header.Set("Content-Type", "application/json")
	if rr := f.do(req, true); rr.Code != http.StatusBadRequest {
		t.Fatalf("non multipart = %d", rr.Code)
	}

	cases := []struct {
		field, name, content string
		chunked              bool
		want                 int
	}{
		{"file", "a.mp4", "x", false, http.StatusBadRequest},                                   // campo errado
		{"video", "a.txt", "x", false, http.StatusUnprocessableEntity},                         // extensão inválida
		{"video", "a.mp4", "", false, http.StatusUnprocessableEntity},                          // vazio
		{"video", "a.mp4", strings.Repeat("x", 2000), false, http.StatusRequestEntityTooLarge}, // Content-Length acima
		{"video", "a.mp4", strings.Repeat("x", 2000), true, http.StatusRequestEntityTooLarge},  // corpo acima (chunked)
	}
	for _, c := range cases {
		body, ct := multipartBody(t, c.field, c.name, c.content)
		req := httptest.NewRequest(http.MethodPost, "/api/videos", body)
		if c.chunked {
			req.ContentLength = -1
		}
		req.Header.Set("Content-Type", ct)
		if rr := f.do(req, true); rr.Code != c.want {
			t.Errorf("%s/%s chunked=%v = %d, want %d (%s)", c.field, c.name, c.chunked, rr.Code, c.want, rr.Body)
		}
	}
}

func TestInternalErrors(t *testing.T) {
	f := newFixture(t, 1<<20)
	f.repo.GetErr = errors.New("db down")
	if rr := f.do(httptest.NewRequest(http.MethodGet, "/api/videos", nil), true); rr.Code != http.StatusInternalServerError {
		t.Fatalf("list infra error = %d", rr.Code)
	}
	if rr := f.do(httptest.NewRequest(http.MethodGet, "/api/videos/x", nil), true); rr.Code != http.StatusInternalServerError {
		t.Fatalf("get infra error = %d", rr.Code)
	}

	f.st.PutErr = errors.New("storage down")
	body, ct := multipartBody(t, "video", "a.mp4", "x")
	req := httptest.NewRequest(http.MethodPost, "/api/videos", body)
	req.Header.Set("Content-Type", ct)
	if rr := f.do(req, true); rr.Code != http.StatusInternalServerError {
		t.Fatalf("upload infra error = %d", rr.Code)
	}
}
