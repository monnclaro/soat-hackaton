// Package httpapi expõe os casos de uso de vídeo via HTTP (controllers, DTOs e métricas de upload).
package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/monnclaro/soat-hackaton/internal/platform/httpx"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
	"github.com/monnclaro/soat-hackaton/internal/platform/storage"
	"github.com/monnclaro/soat-hackaton/internal/video/domain"
	"github.com/monnclaro/soat-hackaton/internal/video/usecase"
)

var uploadsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "fiapx_video_uploads_total",
	Help: "Uploads de vídeo recebidos por resultado.",
}, []string{"result"})

// Handler traduz HTTP ↔ casos de uso.
type Handler struct {
	videos         *usecase.Videos
	tokens         *security.TokenManager
	logger         *slog.Logger
	maxUploadBytes int64
}

// NewHandler cria o handler.
func NewHandler(videos *usecase.Videos, tokens *security.TokenManager, logger *slog.Logger, maxUploadBytes int64) *Handler {
	return &Handler{videos: videos, tokens: tokens, logger: logger, maxUploadBytes: maxUploadBytes}
}

// Register registra as rotas.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/videos", httpx.RequireAuth(h.tokens, h.upload))
	mux.HandleFunc("GET /api/videos", httpx.RequireAuth(h.tokens, h.list))
	mux.HandleFunc("GET /api/videos/{id}", httpx.RequireAuth(h.tokens, h.get))
	mux.HandleFunc("GET /api/videos/{id}/download", httpx.RequireAuth(h.tokens, h.download))
}

// VideoResponse é a representação pública de um vídeo (sem chaves internas de storage).
type VideoResponse struct {
	ID           string    `json:"id"`
	OriginalName string    `json:"original_name"`
	Status       string    `json:"status"`
	SizeBytes    int64     `json:"size_bytes"`
	FrameCount   int       `json:"frame_count"`
	Error        string    `json:"error,omitempty"`
	DownloadURL  string    `json:"download_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func toResponse(v domain.Video) VideoResponse {
	r := VideoResponse{
		ID: v.ID, OriginalName: v.OriginalName, Status: string(v.Status), SizeBytes: v.SizeBytes,
		FrameCount: v.FrameCount, Error: v.ErrorMessage, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
	if v.Downloadable() {
		r.DownloadURL = "/api/videos/" + v.ID + "/download"
	}
	return r
}

func owner(r *http.Request) usecase.Owner {
	p, _ := security.PrincipalFrom(r.Context())
	return usecase.Owner{UserID: p.UserID, Email: p.Email, Name: p.Name}
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > h.maxUploadBytes {
		uploadsTotal.WithLabelValues("rejected").Inc()
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "arquivo excede o limite de "+strconv.FormatInt(h.maxUploadBytes>>20, 10)+" MiB")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)

	// Lê o multipart em streaming: o vídeo vai direto para o storage, sem buffer em disco.
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "envie o vídeo como multipart/form-data no campo 'video'")
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			httpx.WriteError(w, http.StatusBadRequest, "campo 'video' não encontrado")
			return
		}
		if err != nil {
			h.writeUploadErr(w, err)
			return
		}
		if part.FormName() != "video" {
			_ = part.Close()
			continue
		}
		v, err := h.videos.Upload(r.Context(), owner(r), part.FileName(), part)
		_ = part.Close()
		if err != nil {
			h.writeUploadErr(w, err)
			return
		}
		uploadsTotal.WithLabelValues("accepted").Inc()
		w.Header().Set("Location", "/api/videos/"+v.ID)
		httpx.WriteJSON(w, http.StatusAccepted, toResponse(v))
		return
	}
}

func (h *Handler) writeUploadErr(w http.ResponseWriter, err error) {
	if status := statusFor(err); status < 500 {
		uploadsTotal.WithLabelValues("rejected").Inc()
	} else {
		uploadsTotal.WithLabelValues("error").Inc()
	}
	h.writeErr(w, err)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	videos, err := h.videos.List(r.Context(), owner(r).UserID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	out := make([]VideoResponse, 0, len(videos))
	for _, v := range videos {
		out = append(out, toResponse(v))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"videos": out})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	v, err := h.videos.Get(r.Context(), owner(r).UserID, r.PathValue("id"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(v))
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	rc, size, name, err := h.videos.Download(r.Context(), owner(r).UserID, r.PathValue("id"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	defer func() { _ = rc.Close() }()
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, rc); err != nil {
		h.logger.Warn("download interrompido", "error", err)
	}
}

// statusFor mapeia erros de domínio/infraestrutura para status HTTP.
func statusFor(err error) int {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, domain.ErrUnsupportedType), errors.Is(err, domain.ErrEmptyFile), errors.Is(err, domain.ErrInvalidFileName):
		return http.StatusUnprocessableEntity
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, storage.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrNotReady):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	status := statusFor(err)
	switch status {
	case http.StatusRequestEntityTooLarge:
		httpx.WriteError(w, status, "arquivo excede o limite permitido")
	case http.StatusNotFound:
		httpx.WriteError(w, status, domain.ErrNotFound.Error())
	case http.StatusInternalServerError:
		h.logger.Error("erro interno", "error", err)
		httpx.WriteError(w, status, "erro interno")
	default:
		httpx.WriteError(w, status, err.Error())
	}
}
