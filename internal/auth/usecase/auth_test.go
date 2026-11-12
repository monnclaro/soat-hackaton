package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/auth/domain"
)

type memUsers struct {
	byEmail map[string]domain.User
	err     error
}

func newMemUsers() *memUsers { return &memUsers{byEmail: map[string]domain.User{}} }

func (m *memUsers) Create(_ context.Context, u domain.User) error {
	if m.err != nil {
		return m.err
	}
	if _, ok := m.byEmail[u.Email]; ok {
		return domain.ErrEmailTaken
	}
	m.byEmail[u.Email] = u
	return nil
}

func (m *memUsers) FindByEmail(_ context.Context, email string) (domain.User, error) {
	if m.err != nil {
		return domain.User{}, m.err
	}
	u, ok := m.byEmail[email]
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

// plainHasher simula o hasher (o bcrypt real é testado no adaptador).
type plainHasher struct{ err error }

func (h plainHasher) Hash(p string) (string, error) { return "hash:" + p, h.err }
func (plainHasher) Matches(hash, p string) bool     { return hash == "hash:"+p }

type fakeTokens struct {
	last TokenClaims
	err  error
}

func (f *fakeTokens) Issue(c TokenClaims) (string, time.Time, error) {
	f.last = c
	return "token-" + c.UserID, time.Now().Add(time.Hour), f.err
}

func newAuth() (*Auth, *memUsers, *fakeTokens) {
	users, tokens := newMemUsers(), &fakeTokens{}
	a := NewAuth(users, plainHasher{}, tokens)
	a.newID = func() string { return "u1" }
	return a, users, tokens
}

func TestRegisterAndLogin(t *testing.T) {
	a, users, tokens := newAuth()
	ctx := context.Background()

	u, err := a.Register(ctx, " Ana ", " Ana@Example.COM ", "supersecret")
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != "u1" || u.Email != "ana@example.com" || u.Name != "Ana" {
		t.Fatalf("user = %+v", u)
	}
	if users.byEmail["ana@example.com"].PasswordHash != "hash:supersecret" {
		t.Fatal("password must be stored hashed")
	}

	if _, err := a.Register(ctx, "Ana", "ana@example.com", "supersecret"); !errors.Is(err, domain.ErrEmailTaken) {
		t.Fatalf("duplicate: %v", err)
	}

	res, err := a.Login(ctx, "ANA@example.com", "supersecret")
	if err != nil {
		t.Fatal(err)
	}
	if res.AccessToken != "token-u1" || tokens.last.Email != "ana@example.com" || tokens.last.Name != "Ana" {
		t.Fatalf("login = %+v claims=%+v", res, tokens.last)
	}

	if _, err := a.Login(ctx, "ana@example.com", "wrong-pass"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err := a.Login(ctx, "nobody@example.com", "x"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("unknown user: %v", err)
	}

	p, err := a.Profile(ctx, "u1")
	if err != nil || p.Email != "ana@example.com" {
		t.Fatalf("profile = %+v %v", p, err)
	}
}

func TestRegisterErrors(t *testing.T) {
	ctx := context.Background()

	a, _, _ := newAuth()
	var ve domain.ValidationError
	if _, err := a.Register(ctx, "", "a@b.com", "12345678"); !errors.As(err, &ve) {
		t.Fatalf("validation: %v", err)
	}

	a.hasher = plainHasher{err: errors.New("hash failure")}
	if _, err := a.Register(ctx, "Ana", "a@b.com", "12345678"); err == nil {
		t.Fatal("hash error must propagate")
	}

	a, users, _ := newAuth()
	users.err = errors.New("db down")
	if _, err := a.Register(ctx, "Ana", "a@b.com", "12345678"); err == nil {
		t.Fatal("repository error must propagate")
	}
}

func TestLoginInfraErrors(t *testing.T) {
	ctx := context.Background()
	a, users, tokens := newAuth()
	_, _ = a.Register(ctx, "Ana", "a@b.com", "12345678")

	tokens.err = errors.New("signing failure")
	if _, err := a.Login(ctx, "a@b.com", "12345678"); err == nil {
		t.Fatal("token error must propagate")
	}

	users.err = errors.New("db down")
	if _, err := a.Login(ctx, "a@b.com", "12345678"); err == nil || errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("infra error must not look like invalid credentials: %v", err)
	}
}
