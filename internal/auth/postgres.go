package auth

import (
	"context"
	_ "embed"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration é o schema do banco do auth-service (idempotente).
//
//go:embed migrations.sql
var Migration string

// PostgresRepository implementa Repository com PostgreSQL.
type PostgresRepository struct{ pool *pgxpool.Pool }

// NewPostgresRepository cria o repositório.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Create insere o usuário; viola unique de e-mail → ErrEmailTaken.
func (r *PostgresRepository) Create(ctx context.Context, u User) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO users (id, name, email, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		u.ID, u.Name, u.Email, u.PasswordHash, u.CreatedAt)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrEmailTaken
	}
	return err
}

// FindByEmail busca pelo e-mail.
func (r *PostgresRepository) FindByEmail(ctx context.Context, email string) (User, error) {
	return r.findOne(ctx, `SELECT id, name, email, password_hash, created_at FROM users WHERE email = $1`, email)
}

// FindByID busca pelo id.
func (r *PostgresRepository) FindByID(ctx context.Context, id string) (User, error) {
	return r.findOne(ctx, `SELECT id, name, email, password_hash, created_at FROM users WHERE id = $1`, id)
}

func (r *PostgresRepository) findOne(ctx context.Context, q string, arg any) (User, error) {
	var u User
	err := r.pool.QueryRow(ctx, q, arg).Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}
