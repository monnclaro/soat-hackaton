package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	authbcrypt "github.com/monnclaro/soat-hackaton/internal/auth/adapter/bcrypt"
	"github.com/monnclaro/soat-hackaton/internal/auth/adapter/jwt"
	"github.com/monnclaro/soat-hackaton/internal/auth/domain"
	"github.com/monnclaro/soat-hackaton/internal/auth/usecase"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

type memUsers struct {
	byEmail map[string]domain.User
	err     error
}

func (m *memUsers) Create(_ context.Context, u domain.User) error {
	if _, ok := m.byEmail[u.Email]; ok {
		return domain.ErrEmailTaken
	}
	m.byEmail[u.Email] = u
	return nil
}

func (m *memUsers) FindByEmail(_ context.Context, e string) (domain.User, error) {
	if m.err != nil {
		return domain.User{}, m.err
	}
	u, ok := m.byEmail[e]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, nil
}

func (m *memUsers) FindByID(_ context.Context, id string) (domain.User, error) {
	for _, u := range m.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrUserNotFound
}

func TestHTTPFlow(t *testing.T) {
	tm, _ := security.NewTokenManager("0123456789abcdef0123456789abcdef", time.Hour)
	users := &memUsers{byEmail: map[string]domain.User{}}
	auth := usecase.NewAuth(users, authbcrypt.New(bcrypt.MinCost), jwt.New(tm))
	mux := http.NewServeMux()
	NewHandler(auth, tm, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	do := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	rr := do("POST", "/auth/register", `{"name":"Ana","email":"ana@x.com","password":"12345678"}`, "")
	if rr.Code != http.StatusCreated {
		t.Fatalf("register = %d %s", rr.Code, rr.Body)
	}
	if strings.Contains(rr.Body.String(), "password") || strings.Contains(rr.Body.String(), "hash") {
		t.Fatal("response must not leak the password hash")
	}
	for _, c := range []struct {
		body string
		want int
	}{
		{`{"name":"Ana","email":"ana@x.com","password":"12345678"}`, http.StatusConflict},
		{`{"name":"","email":"ana@x.com","password":"12345678"}`, http.StatusUnprocessableEntity},
		{`not json`, http.StatusBadRequest},
	} {
		if rr := do("POST", "/auth/register", c.body, ""); rr.Code != c.want {
			t.Fatalf("register %s = %d, want %d", c.body, rr.Code, c.want)
		}
	}

	if rr := do("POST", "/auth/login", `{"email":"ana@x.com","password":"nope-nope"}`, ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d", rr.Code)
	}
	if rr := do("POST", "/auth/login", `{`, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad login json = %d", rr.Code)
	}

	rr = do("POST", "/auth/login", `{"email":"ana@x.com","password":"12345678"}`, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("login = %d %s", rr.Code, rr.Body)
	}
	var res LoginResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	if res.AccessToken == "" || res.TokenType != "Bearer" || res.User.Email != "ana@x.com" {
		t.Fatalf("login body %+v", res)
	}

	if rr := do("GET", "/auth/me", "", res.AccessToken); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "ana@x.com") {
		t.Fatalf("me = %d %s", rr.Code, rr.Body)
	}
	if rr := do("GET", "/auth/me", "", ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("me without token = %d", rr.Code)
	}
	ghost, _, _ := tm.Issue(security.Principal{UserID: "ghost"})
	if rr := do("GET", "/auth/me", "", ghost); rr.Code != http.StatusNotFound {
		t.Fatalf("me ghost = %d", rr.Code)
	}

	users.err = errors.New("db down")
	if rr := do("POST", "/auth/login", `{"email":"ana@x.com","password":"12345678"}`, ""); rr.Code != http.StatusInternalServerError {
		t.Fatalf("infra error = %d", rr.Code)
	}
}
