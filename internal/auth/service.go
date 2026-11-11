// Package auth implementa o cadastro e a autenticação de usuários (usuário e senha → JWT).
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

// Erros de domínio.
var (
	ErrEmailTaken         = errors.New("e-mail já cadastrado")
	ErrInvalidCredentials = errors.New("e-mail ou senha inválidos")
	ErrNotFound           = errors.New("usuário não encontrado")
)

// ValidationError indica entrada inválida.
type ValidationError struct{ Msg string }

func (e ValidationError) Error() string { return e.Msg }

// User é o agregado de usuário.
type User struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Repository é a porta de persistência de usuários.
type Repository interface {
	Create(ctx context.Context, u User) error
	FindByEmail(ctx context.Context, email string) (User, error)
	FindByID(ctx context.Context, id string) (User, error)
}

// TokenIssuer emite tokens de acesso.
type TokenIssuer interface {
	Issue(p security.Principal) (string, time.Time, error)
}

// Service contém os casos de uso de autenticação.
type Service struct {
	repo       Repository
	tokens     TokenIssuer
	bcryptCost int
	now        func() time.Time
}

// NewService cria o serviço. bcryptCost <= 0 usa o custo padrão.
func NewService(repo Repository, tokens TokenIssuer, bcryptCost int) *Service {
	if bcryptCost <= 0 {
		bcryptCost = bcrypt.DefaultCost
	}
	return &Service{repo: repo, tokens: tokens, bcryptCost: bcryptCost, now: time.Now}
}

// Register cadastra um novo usuário.
func (s *Service) Register(ctx context.Context, name, email, password string) (User, error) {
	name = strings.TrimSpace(name)
	email = normalizeEmail(email)
	if name == "" || len(name) > 120 {
		return User{}, ValidationError{"nome é obrigatório (até 120 caracteres)"}
	}
	if _, err := mail.ParseAddress(email); err != nil || len(email) > 254 {
		return User{}, ValidationError{"e-mail inválido"}
	}
	if len(password) < 8 || len(password) > 72 {
		return User{}, ValidationError{"senha deve ter entre 8 e 72 caracteres"}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return User{}, fmt.Errorf("gerando hash da senha: %w", err)
	}
	u := User{ID: uuid.NewString(), Name: name, Email: email, PasswordHash: string(hash), CreatedAt: s.now().UTC()}
	if err := s.repo.Create(ctx, u); err != nil {
		return User{}, err
	}
	return u, nil
}

// LoginResult é o retorno do login.
type LoginResult struct {
	AccessToken string    `json:"access_token"`
	TokenType   string    `json:"token_type"`
	ExpiresAt   time.Time `json:"expires_at"`
	User        User      `json:"user"`
}

// Login valida as credenciais e emite um JWT.
func (s *Service) Login(ctx context.Context, email, password string) (LoginResult, error) {
	u, err := s.repo.FindByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, ErrNotFound) {
		return LoginResult{}, ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return LoginResult{}, ErrInvalidCredentials
	}
	tok, exp, err := s.tokens.Issue(security.Principal{UserID: u.ID, Email: u.Email, Name: u.Name})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: tok, TokenType: "Bearer", ExpiresAt: exp, User: u}, nil
}

// Me retorna o usuário autenticado.
func (s *Service) Me(ctx context.Context, id string) (User, error) {
	return s.repo.FindByID(ctx, id)
}

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }
