// Package postgres implementa a porta UserRepository com PostgreSQL.
package postgres

import (
	"context"
	_ "embed"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monnclaro/soat-hackaton/internal/auth/domain"
)

// Migration é o schema do banco do auth-service (idempotente).
//
//go:embed migrations.sql
var Migration string

// UserRepository persiste usuários no banco fiapx_auth.
type UserRepository struct{ pool *pgxpool.Pool }

// NewUserRepository cria o repositório.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create insere o usuário; violação do índice único de e-mail vira domain.ErrEmailTaken.
func (r *UserRepository) Create(ctx context.Context, u domain.User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, name, email, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Name, u.Email, u.PasswordHash, u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrEmailTaken
	}
	return err
}

// FindByEmail busca pelo e-mail.
func (r *UserRepository) FindByEmail(ctx context.Context, email string) (domain.User, error) {
	return r.findOne(ctx, `SELECT id, name, email, password_hash, created_at FROM users WHERE email = $1`, email)
}

// FindByID busca pelo id.
func (r *UserRepository) FindByID(ctx context.Context, id string) (domain.User, error) {
	if _, err := uuid.Parse(id); err != nil {
		return domain.User{}, domain.ErrUserNotFound
	}
	return r.findOne(ctx, `SELECT id, name, email, password_hash, created_at FROM users WHERE id = $1`, id)
}

func (r *UserRepository) findOne(ctx context.Context, q string, arg any) (domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx, q, arg).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	return u, err
}
