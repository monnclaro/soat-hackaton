package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

type memRepo struct {
	mu      sync.Mutex
	byEmail map[string]User
	err     error
}

func newMemRepo() *memRepo { return &memRepo{byEmail: map[string]User{}} }

func (m *memRepo) Create(_ context.Context, u User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	if _, ok := m.byEmail[u.Email]; ok {
		return ErrEmailTaken
	}
	m.byEmail[u.Email] = u
	return nil
}

func (m *memRepo) FindByEmail(_ context.Context, email string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return User{}, m.err
	}
	u, ok := m.byEmail[email]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

func (m *memRepo) FindByID(_ context.Context, id string) (User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.byEmail {
		if u.ID == id {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

func newTestService(t *testing.T) (*Service, *memRepo, *security.TokenManager) {
	t.Helper()
	tm, err := security.NewTokenManager("0123456789abcdef0123456789abcdef", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	repo := newMemRepo()
	return NewService(repo, tm, bcrypt.MinCost), repo, tm
}

func TestRegisterValidation(t *testing.T) {
	svc, _, _ := newTestService(t)
	cases := []struct{ name, email, pwd string }{
		{"", "a@b.com", "12345678"},
		{strings.Repeat("x", 121), "a@b.com", "12345678"},
		{"Ana", "not-an-email", "12345678"},
		{"Ana", "a@b.com", "short"},
		{"Ana", "a@b.com", strings.Repeat("x", 73)},
	}
	for _, c := range cases {
		_, err := svc.Register(context.Background(), c.name, c.email, c.pwd)
		var ve ValidationError
		if !errors.As(err, &ve) {
			t.Fatalf("Register(%q,%q) expected ValidationError, got %v", c.name, c.email, err)
		}
	}
}

func TestRegisterAndLogin(t *testing.T) {
	svc, repo, tm := newTestService(t)
	ctx := context.Background()

	u, err := svc.Register(ctx, " Ana ", " Ana@Example.COM ", "supersecret")
	if err != nil {
		t.Fatal(err)
	}
	if u.Email != "ana@example.com" || u.Name != "Ana" || u.ID == "" {
		t.Fatalf("unexpected user %+v", u)
	}
	if repo.byEmail["ana@example.com"].PasswordHash == "supersecret" {
		t.Fatal("password must be hashed")
	}

	if _, err := svc.Register(ctx, "Ana", "ana@example.com", "supersecret"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate: got %v", err)
	}

	res, err := svc.Login(ctx, "ANA@example.com", "supersecret")
	if err != nil {
		t.Fatal(err)
	}
	p, err := tm.Verify(res.AccessToken)
	if err != nil || p.UserID != u.ID || p.Email != u.Email {
		t.Fatalf("token principal %+v err %v", p, err)
	}

	if _, err := svc.Login(ctx, "ana@example.com", "wrongpass"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: got %v", err)
	}
	if _, err := svc.Login(ctx, "nobody@example.com", "x"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user: got %v", err)
	}

	me, err := svc.Me(ctx, u.ID)
	if err != nil || me.Email != u.Email {
		t.Fatalf("Me: %+v %v", me, err)
	}
}

func TestLoginPropagatesRepositoryError(t *testing.T) {
	svc, repo, _ := newTestService(t)
	repo.err = errors.New("db down")
	if _, err := svc.Login(context.Background(), "a@b.com", "x"); err == nil || errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected infra error, got %v", err)
	}
}

func TestHTTPFlow(t *testing.T) {
	svc, repo, tm := newTestService(t)
	mux := http.NewServeMux()
	NewHandler(svc, tm, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(mux)

	do := func(method, path, body, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		return rr
	}

	if rr := do("POST", "/auth/register", `{"name":"Ana","email":"ana@x.com","password":"12345678"}`, ""); rr.Code != http.StatusCreated {
		t.Fatalf("register = %d %s", rr.Code, rr.Body)
	} else if strings.Contains(rr.Body.String(), "password") {
		t.Fatal("response must not leak password hash")
	}
	if rr := do("POST", "/auth/register", `{"name":"Ana","email":"ana@x.com","password":"12345678"}`, ""); rr.Code != http.StatusConflict {
		t.Fatalf("duplicate = %d", rr.Code)
	}
	if rr := do("POST", "/auth/register", `{"name":"","email":"ana@x.com","password":"12345678"}`, ""); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid = %d", rr.Code)
	}
	if rr := do("POST", "/auth/register", `not json`, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad json = %d", rr.Code)
	}
	if rr := do("POST", "/auth/login", `{"email":"ana@x.com","password":"nope-nope"}`, ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("bad login = %d", rr.Code)
	}
	if rr := do("POST", "/auth/login", `{`, ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("bad login json = %d", rr.Code)
	}

	rr := do("POST", "/auth/login", `{"email":"ana@x.com","password":"12345678"}`, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("login = %d %s", rr.Code, rr.Body)
	}
	var res LoginResult
	_ = json.Unmarshal(rr.Body.Bytes(), &res)
	if res.AccessToken == "" || res.TokenType != "Bearer" {
		t.Fatalf("login body %+v", res)
	}

	if rr := do("GET", "/auth/me", "", res.AccessToken); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "ana@x.com") {
		t.Fatalf("me = %d %s", rr.Code, rr.Body)
	}
	if rr := do("GET", "/auth/me", "", ""); rr.Code != http.StatusUnauthorized {
		t.Fatalf("me without token = %d", rr.Code)
	}

	// usuário do token não existe mais → 404; erro de infraestrutura → 500
	ghost, _, _ := tm.Issue(security.Principal{UserID: "ghost"})
	if rr := do("GET", "/auth/me", "", ghost); rr.Code != http.StatusNotFound {
		t.Fatalf("me ghost = %d", rr.Code)
	}
	repo.err = errors.New("db down")
	if rr := do("POST", "/auth/login", `{"email":"ana@x.com","password":"12345678"}`, ""); rr.Code != http.StatusInternalServerError {
		t.Fatalf("infra error = %d", rr.Code)
	}
}
