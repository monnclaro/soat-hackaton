package video

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Migration é o schema do banco do video-service (idempotente).
//
//go:embed migrations.sql
var Migration string

// PostgresRepository implementa Repository e OutboxStore com PostgreSQL.
type PostgresRepository struct{ pool *pgxpool.Pool }

// NewPostgresRepository cria o repositório.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

const videoColumns = `id, user_id, user_email, original_name, storage_key, COALESCE(zip_key, ''), size_bytes,
	frame_count, status, COALESCE(error_message, ''), created_at, updated_at`

func scanVideo(row pgx.Row) (Video, error) {
	var v Video
	err := row.Scan(&v.ID, &v.UserID, &v.UserEmail, &v.OriginalName, &v.StorageKey, &v.ZipKey, &v.SizeBytes,
		&v.FrameCount, &v.Status, &v.ErrorMessage, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

// CreateWithOutbox grava vídeo + mensagem de outbox na mesma transação.
func (r *PostgresRepository) CreateWithOutbox(ctx context.Context, v Video, msg OutboxMessage) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO videos (id, user_id, user_email, original_name, storage_key, size_bytes,
			frame_count, status, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			v.ID, v.UserID, v.UserEmail, v.OriginalName, v.StorageKey, v.SizeBytes, v.FrameCount, v.Status,
			v.CreatedAt, v.UpdatedAt); err != nil {
			return fmt.Errorf("inserindo vídeo: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO outbox (id, routing_key, payload, created_at) VALUES ($1,$2,$3,$4)`,
			msg.ID, msg.RoutingKey, msg.Payload, msg.CreatedAt); err != nil {
			return fmt.Errorf("inserindo outbox: %w", err)
		}
		return nil
	})
}

// Get busca um vídeo pelo id.
func (r *PostgresRepository) Get(ctx context.Context, id string) (Video, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Video{}, ErrNotFound
	}
	v, err := scanVideo(r.pool.QueryRow(ctx, `SELECT `+videoColumns+` FROM videos WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Video{}, ErrNotFound
	}
	return v, err
}

// ListByUser lista os vídeos do usuário, mais recentes primeiro.
func (r *PostgresRepository) ListByUser(ctx context.Context, userID string) ([]Video, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+videoColumns+` FROM videos WHERE user_id = $1 ORDER BY created_at DESC LIMIT 500`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Video{}
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Update persiste status e resultado do processamento.
func (r *PostgresRepository) Update(ctx context.Context, v Video) error {
	tag, err := r.pool.Exec(ctx, `UPDATE videos SET status=$2, zip_key=NULLIF($3,''), frame_count=$4,
		error_message=NULLIF($5,''), updated_at=$6 WHERE id=$1`,
		v.ID, v.Status, v.ZipKey, v.FrameCount, v.ErrorMessage, v.UpdatedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DispatchPending publica até `limit` mensagens pendentes. Usa FOR UPDATE SKIP LOCKED para que
// várias réplicas do video-service possam rodar o relay em paralelo sem publicar a mesma mensagem.
// Marca como publicadas apenas as mensagens confirmadas pelo broker.
func (r *PostgresRepository) DispatchPending(ctx context.Context, limit int, publish func(context.Context, OutboxMessage) error) (int, error) {
	sent := 0
	var pubErr error
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, routing_key, payload, created_at FROM outbox
			WHERE published_at IS NULL ORDER BY created_at LIMIT $1 FOR UPDATE SKIP LOCKED`, limit)
		if err != nil {
			return err
		}
		var pending []OutboxMessage
		for rows.Next() {
			var m OutboxMessage
			if err := rows.Scan(&m.ID, &m.RoutingKey, &m.Payload, &m.CreatedAt); err != nil {
				rows.Close()
				return err
			}
			pending = append(pending, m)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}

		var published []string
		for _, m := range pending {
			// Em caso de falha, interrompe: o que já foi publicado é confirmado no commit
			// e o restante é tentado no próximo ciclo.
			if pubErr = publish(ctx, m); pubErr != nil {
				break
			}
			published = append(published, m.ID)
		}
		if len(published) > 0 {
			if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = ANY($1::uuid[])`, published); err != nil {
				return err
			}
		}
		sent = len(published)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return sent, pubErr
}

// Ping verifica a conexão (readiness).
func (r *PostgresRepository) Ping(ctx context.Context) error { return r.pool.Ping(ctx) }
