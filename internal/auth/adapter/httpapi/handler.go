// Package httpapi expõe os casos de uso de identidade via HTTP (controllers + DTOs).
package httpapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/auth/domain"
	"github.com/monnclaro/soat-hackaton/internal/auth/usecase"
	"github.com/monnclaro/soat-hackaton/internal/platform/httpx"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

// Handler traduz HTTP ↔ casos de uso.
type Handler struct {
	auth   *usecase.Auth
	tokens *security.TokenManager
	logger *slog.Logger
}

// NewHandler cria o handler.
func NewHandler(auth *usecase.Auth, tokens *security.TokenManager, logger *slog.Logger) *Handler {
	return &Handler{auth: auth, tokens: tokens, logger: logger}
}

// Register registra as rotas no mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	mux.HandleFunc("GET /auth/me", httpx.RequireAuth(h.tokens, h.me))
}

type registerRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UserResponse é a representação pública do usuário (sem o hash da senha).
type UserResponse struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// LoginResponse é o corpo da resposta do login.
type LoginResponse struct {
	AccessToken string       `json:"access_token"`
	TokenType   string       `json:"token_type"`
	ExpiresAt   time.Time    `json:"expires_at"`
	User        UserResponse `json:"user"`
}

func toUserResponse(u domain.User) UserResponse {
	return UserResponse{ID: u.ID, Name: u.Name, Email: u.Email, CreatedAt: u.CreatedAt}
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.DecodeJSON(r, w, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	u, err := h.auth.Register(r.Context(), req.Name, req.Email, req.Password)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toUserResponse(u))
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(r, w, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "JSON inválido")
		return
	}
	res, err := h.auth.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, LoginResponse{
		AccessToken: res.AccessToken, TokenType: "Bearer", ExpiresAt: res.ExpiresAt, User: toUserResponse(res.User),
	})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	p, _ := security.PrincipalFrom(r.Context())
	u, err := h.auth.Profile(r.Context(), p.UserID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, toUserResponse(u))
}

// writeErr mapeia erros de domínio para status HTTP.
func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	var ve domain.ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, http.StatusUnprocessableEntity, ve.Msg)
	case errors.Is(err, domain.ErrEmailTaken):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		httpx.WriteError(w, http.StatusNotFound, err.Error())
	default:
		h.logger.Error("erro interno", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "erro interno")
	}
}
