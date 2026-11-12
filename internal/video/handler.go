package video

import (
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/httpx"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
	"github.com/monnclaro/soat-hackaton/internal/platform/storage"
)

// Handler expõe a API de vídeos.
type Handler struct {
	svc            *Service
	tokens         *security.TokenManager
	logger         *slog.Logger
	maxUploadBytes int64
}

// NewHandler cria o handler HTTP.
func NewHandler(svc *Service, tokens *security.TokenManager, logger *slog.Logger, maxUploadBytes int64) *Handler {
	return &Handler{svc: svc, tokens: tokens, logger: logger, maxUploadBytes: maxUploadBytes}
}

// Register registra as rotas.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/videos", httpx.RequireAuth(h.tokens, h.upload))
	mux.HandleFunc("GET /api/videos", httpx.RequireAuth(h.tokens, h.list))
	mux.HandleFunc("GET /api/videos/{id}", httpx.RequireAuth(h.tokens, h.get))
	mux.HandleFunc("GET /api/videos/{id}/download", httpx.RequireAuth(h.tokens, h.download))
}

// Response é a representação pública de um vídeo.
type Response struct {
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

func toResponse(v Video) Response {
	r := Response{
		ID: v.ID, OriginalName: v.OriginalName, Status: v.Status, SizeBytes: v.SizeBytes,
		FrameCount: v.FrameCount, Error: v.ErrorMessage, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt,
	}
	if v.Status == events.StatusCompleted {
		r.DownloadURL = "/api/videos/" + v.ID + "/download"
	}
	return r
}

func (h *Handler) upload(w http.ResponseWriter, r *http.Request) {
	p, _ := security.PrincipalFrom(r.Context())
	if r.ContentLength > h.maxUploadBytes {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "arquivo excede o limite de "+strconv.FormatInt(h.maxUploadBytes>>20, 10)+" MiB")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUploadBytes)

	// Lê o multipart em streaming: o vídeo vai direto para o object storage, sem buffer em disco.
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
			h.writeErr(w, err)
			return
		}
		if part.FormName() != "video" {
			_ = part.Close()
			continue
		}
		v, err := h.svc.Upload(r.Context(), p, part.FileName(), part)
		_ = part.Close()
		if err != nil {
			h.writeErr(w, err)
			return
		}
		w.Header().Set("Location", "/api/videos/"+v.ID)
		httpx.WriteJSON(w, http.StatusAccepted, toResponse(v))
		return
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	p, _ := security.PrincipalFrom(r.Context())
	videos, err := h.svc.List(r.Context(), p.UserID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	out := make([]Response, 0, len(videos))
	for _, v := range videos {
		out = append(out, toResponse(v))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"videos": out})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	p, _ := security.PrincipalFrom(r.Context())
	v, err := h.svc.Get(r.Context(), p.UserID, r.PathValue("id"))
	if err != nil {
		h.writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toResponse(v))
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request) {
	p, _ := security.PrincipalFrom(r.Context())
	rc, size, name, err := h.svc.Download(r.Context(), p.UserID, r.PathValue("id"))
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

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "arquivo excede o limite permitido")
	case errors.Is(err, ErrUnsupportedType), errors.Is(err, ErrEmptyFile), errors.Is(err, ErrInvalidFileName):
		httpx.WriteError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrNotFound), errors.Is(err, storage.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, ErrNotFound.Error())
	case errors.Is(err, ErrNotReady):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	default:
		h.logger.Error("erro interno", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "erro interno")
	}
}
