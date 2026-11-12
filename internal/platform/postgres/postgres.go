// Package postgres cria o pool de conexões e aplica migrações idempotentes.
package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Connect abre o pool e aguarda o banco ficar disponível (com retentativas) até o ctx expirar.
func Connect(ctx context.Context, url string, logger *slog.Logger) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	backoff := 500 * time.Millisecond
	for {
		err = pool.Ping(ctx)
		if err == nil {
			return pool, nil
		}
		logger.Warn("postgres indisponível, tentando novamente", "error", err, "retry_in", backoff.String())
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, fmt.Errorf("conectando ao postgres: %w", err)
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 10*time.Second)
	}
}

// Migrate executa o script SQL (deve ser idempotente: CREATE ... IF NOT EXISTS).
// Usa advisory lock para que várias réplicas subindo juntas não conflitem.
func Migrate(ctx context.Context, pool *pgxpool.Pool, script string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	const lockID = 727274 // "FIAPX"
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", lockID); err != nil {
		return fmt.Errorf("obtendo lock de migração: %w", err)
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", lockID) }()
	if _, err := conn.Exec(ctx, script); err != nil {
		return fmt.Errorf("aplicando migração: %w", err)
	}
	return nil
}
