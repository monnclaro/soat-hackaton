// Package httpx reúne utilitários HTTP compartilhados: respostas JSON, middlewares e servidor com graceful shutdown.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

// ErrorBody é o formato padrão de erro da API.
type ErrorBody struct {
	Error string `json:"error"`
}

// WriteJSON serializa v como JSON com o status informado.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// WriteError escreve um erro no formato padrão.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, ErrorBody{Error: msg})
}

// DecodeJSON lê o corpo como JSON (limitado a 1 MiB) rejeitando campos desconhecidos.
func DecodeJSON(r *http.Request, w http.ResponseWriter, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush permite streaming de respostas (download).
func (s *statusRecorder) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap expõe o writer original ao http.ResponseController.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total de requisições HTTP.",
	}, []string{"route", "method", "status"})
	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "Latência das requisições HTTP.",
		Buckets: prometheus.DefBuckets,
	}, []string{"route", "method"})
)

// Observe aplica recover, log de acesso e métricas Prometheus.
// A rota é o padrão do ServeMux (r.Pattern), evitando alta cardinalidade.
func Observe(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				logger.Error("panic na requisição", "panic", p, "path", r.URL.Path)
				rec.WriteHeader(http.StatusInternalServerError)
			}
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			elapsed := time.Since(start)
			httpRequests.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Inc()
			httpDuration.WithLabelValues(route, r.Method).Observe(elapsed.Seconds())
			if route != "GET /healthz" && route != "GET /metrics" {
				logger.Info("http", "method", r.Method, "route", route, "status", rec.status, "duration_ms", elapsed.Milliseconds())
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

// RequireAuth valida o header "Authorization: Bearer <token>".
func RequireAuth(tm *security.TokenManager, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(h, "Bearer ")
		if !ok || token == "" {
			WriteError(w, http.StatusUnauthorized, "autenticação necessária")
			return
		}
		p, err := tm.Verify(token)
		if err != nil {
			WriteError(w, http.StatusUnauthorized, "token inválido ou expirado")
			return
		}
		next(w, r.WithContext(security.WithPrincipal(r.Context(), p)))
	}
}

// RegisterOps registra /healthz (liveness), /readyz (readiness) e /metrics.
func RegisterOps(mux *http.ServeMux, ready func(context.Context) error) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				WriteError(w, http.StatusServiceUnavailable, err.Error())
				return
			}
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.Handle("GET /metrics", promhttp.Handler())
}

// Serve inicia o servidor e faz graceful shutdown quando ctx é cancelado.
func Serve(ctx context.Context, logger *slog.Logger, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("servidor HTTP iniciado", "addr", addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		logger.Info("encerrando servidor HTTP")
		return srv.Shutdown(shutdownCtx)
	}
}
