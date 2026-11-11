package httpx

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRequireAuth(t *testing.T) {
	tm, _ := security.NewTokenManager("0123456789abcdef0123456789abcdef", time.Hour)
	tok, _, _ := tm.Issue(security.Principal{UserID: "u1", Email: "e@x.com"})

	h := RequireAuth(tm, func(w http.ResponseWriter, r *http.Request) {
		p, _ := security.PrincipalFrom(r.Context())
		_, _ = w.Write([]byte(p.UserID))
	})

	cases := []struct {
		name, header string
		want         int
	}{
		{"sem header", "", http.StatusUnauthorized},
		{"esquema errado", "Basic abc", http.StatusUnauthorized},
		{"token inválido", "Bearer abc", http.StatusUnauthorized},
		{"ok", "Bearer " + tok, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if c.header != "" {
				req.Header.Set("Authorization", c.header)
			}
			rr := httptest.NewRecorder()
			h(rr, req)
			if rr.Code != c.want {
				t.Fatalf("status = %d, want %d", rr.Code, c.want)
			}
			if c.want == http.StatusOK && rr.Body.String() != "u1" {
				t.Fatalf("body = %q", rr.Body.String())
			}
		})
	}
}

func TestObserveRecoversPanicAndUsesPattern(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("x") })
	mux.HandleFunc("GET /ok/{id}", func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, 201, map[string]string{"id": r.PathValue("id")})
	})
	h := Observe(discardLogger(), mux)

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ok/7", nil))
	if rr.Code != 201 || !strings.Contains(rr.Body.String(), `"id":"7"`) {
		t.Fatalf("got %d %s", rr.Code, rr.Body.String())
	}
}

func TestOpsEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	fail := true
	RegisterOps(mux, func(context.Context) error {
		if fail {
			return errors.New("db down")
		}
		return nil
	})

	for _, tc := range []struct {
		path string
		want int
	}{{"/healthz", 200}, {"/readyz", 503}, {"/metrics", 200}} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rr.Code != tc.want {
			t.Fatalf("%s = %d, want %d", tc.path, rr.Code, tc.want)
		}
	}
	fail = false
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rr.Code != 200 {
		t.Fatalf("readyz = %d", rr.Code)
	}
}

func TestDecodeJSONRejectsUnknownFields(t *testing.T) {
	var dst struct {
		A string `json:"a"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"1","b":2}`))
	if err := DecodeJSON(req, httptest.NewRecorder(), &dst); err == nil {
		t.Fatal("expected error for unknown field")
	}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"1"}`))
	if err := DecodeJSON(req, httptest.NewRecorder(), &dst); err != nil || dst.A != "1" {
		t.Fatalf("decode: %v %+v", err, dst)
	}
}

func TestServeShutsDownOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, discardLogger(), "127.0.0.1:0", http.NewServeMux()) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not shut down")
	}
}
