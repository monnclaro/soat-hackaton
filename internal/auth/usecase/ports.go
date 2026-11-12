// Package usecase contém os casos de uso de identidade (cadastro, login, perfil).
// Depende apenas do domínio e das portas (interfaces) declaradas aqui; as implementações
// concretas (Postgres, bcrypt, JWT) ficam nos adaptadores e são injetadas pelo cmd/.
package usecase

import (
	"context"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/auth/domain"
)

// UserRepository persiste usuários.
type UserRepository interface {
	// Create retorna domain.ErrEmailTaken se o e-mail já existir.
	Create(ctx context.Context, u domain.User) error
	// FindByEmail e FindByID retornam domain.ErrUserNotFound se não existir.
	FindByEmail(ctx context.Context, email string) (domain.User, error)
	FindByID(ctx context.Context, id string) (domain.User, error)
}

// PasswordHasher gera e confere hashes de senha.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Matches(hash, password string) bool
}

// TokenClaims são os dados do usuário gravados no token de acesso.
type TokenClaims struct {
	UserID string
	Email  string
	Name   string
}

// TokenIssuer emite tokens de acesso.
type TokenIssuer interface {
	Issue(claims TokenClaims) (token string, expiresAt time.Time, err error)
}
