-- Schema do video-service (banco: fiapx_videos). Idempotente.
CREATE TABLE IF NOT EXISTS videos (
    id            UUID PRIMARY KEY,
    user_id       UUID         NOT NULL,
    user_email    VARCHAR(254) NOT NULL,
    original_name VARCHAR(255) NOT NULL,
    storage_key   TEXT         NOT NULL,
    zip_key       TEXT,
    size_bytes    BIGINT       NOT NULL DEFAULT 0,
    frame_count   INTEGER      NOT NULL DEFAULT 0,
    status        VARCHAR(20)  NOT NULL,
    error_message TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT ck_videos_status CHECK (status IN ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED'))
);

CREATE INDEX IF NOT EXISTS ix_videos_user_created ON videos (user_id, created_at DESC);

-- Transactional outbox: mensagens gravadas na mesma transação do vídeo e publicadas
-- de forma assíncrona no RabbitMQ (garante que nenhum upload aceito deixe de ser processado).
CREATE TABLE IF NOT EXISTS outbox (
    id           UUID PRIMARY KEY,
    routing_key  VARCHAR(100) NOT NULL,
    payload      JSONB        NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ix_outbox_pending ON outbox (created_at) WHERE published_at IS NULL;
