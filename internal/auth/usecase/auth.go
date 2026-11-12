package usecase

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/monnclaro/soat-hackaton/internal/auth/domain"
)

// Auth agrupa os casos de uso de identidade.
type Auth struct {
	users  UserRepository
	hasher PasswordHasher
	tokens TokenIssuer
	now    func() time.Time
	newID  func() string
}

// NewAuth cria os casos de uso com as dependências injetadas.
func NewAuth(users UserRepository, hasher PasswordHasher, tokens TokenIssuer) *Auth {
	return &Auth{users: users, hasher: hasher, tokens: tokens, now: time.Now, newID: uuid.NewString}
}

// Register cadastra um novo usuário.
func (a *Auth) Register(ctx context.Context, name, email, password string) (domain.User, error) {
	reg, err := domain.NewRegistration(name, email, password)
	if err != nil {
		return domain.User{}, err
	}
	hash, err := a.hasher.Hash(reg.Password)
	if err != nil {
		return domain.User{}, fmt.Errorf("gerando hash da senha: %w", err)
	}
	u := domain.User{ID: a.newID(), Name: reg.Name, Email: reg.Email, PasswordHash: hash, CreatedAt: a.now().UTC()}
	if err := a.users.Create(ctx, u); err != nil {
		return domain.User{}, err
	}
	return u, nil
}

// LoginResult é o resultado de um login bem-sucedido.
type LoginResult struct {
	AccessToken string
	ExpiresAt   time.Time
	User        domain.User
}

// Login confere as credenciais e emite um token de acesso.
// Usuário inexistente e senha errada retornam o mesmo erro, para não revelar quais e-mails existem.
func (a *Auth) Login(ctx context.Context, email, password string) (LoginResult, error) {
	u, err := a.users.FindByEmail(ctx, domain.NormalizeEmail(email))
	if errors.Is(err, domain.ErrUserNotFound) {
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return LoginResult{}, err
	}
	if !a.hasher.Matches(u.PasswordHash, password) {
		return LoginResult{}, domain.ErrInvalidCredentials
	}
	token, exp, err := a.tokens.Issue(TokenClaims{UserID: u.ID, Email: u.Email, Name: u.Name})
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{AccessToken: token, ExpiresAt: exp, User: u}, nil
}

// Profile retorna os dados do usuário autenticado.
func (a *Auth) Profile(ctx context.Context, userID string) (domain.User, error) {
	return a.users.FindByID(ctx, userID)
}
