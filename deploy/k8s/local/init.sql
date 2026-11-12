-- =============================================================================
-- FIAP X - Script de criação dos bancos de dados (PostgreSQL 17)
--
-- Database-per-service: cada microsserviço tem o seu banco e só ele o acessa.
--   fiapx_auth   → auth-service  (usuários)
--   fiapx_videos → video-service (vídeos + outbox)
--
-- Executado automaticamente pelo container do Postgres no docker-compose
-- (/docker-entrypoint-initdb.d). Os serviços também aplicam o mesmo DDL de forma
-- idempotente ao subir (internal/*/migrations.sql), o que permite usar um banco
-- gerenciado (de qualquer provedor) sem rodar este script manualmente.
-- =============================================================================

CREATE DATABASE fiapx_auth;
CREATE DATABASE fiapx_videos;

-- -----------------------------------------------------------------------------
-- auth-service
-- -----------------------------------------------------------------------------
\connect fiapx_auth

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY,
    name          VARCHAR(120) NOT NULL,
    email         VARCHAR(254) NOT NULL,
    password_hash VARCHAR(100) NOT NULL,          -- bcrypt
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_users_email ON users (email);

-- -----------------------------------------------------------------------------
-- video-service
-- -----------------------------------------------------------------------------
\connect fiapx_videos

CREATE TABLE IF NOT EXISTS videos (
    id            UUID PRIMARY KEY,
    user_id       UUID         NOT NULL,          -- referência lógica a fiapx_auth.users (sem FK entre bancos)
    user_email    VARCHAR(254) NOT NULL,          -- usado na notificação de falha
    original_name VARCHAR(255) NOT NULL,
    storage_key   TEXT         NOT NULL,          -- objeto do vídeo no S3/MinIO
    zip_key       TEXT,                           -- objeto do ZIP de frames
    size_bytes    BIGINT       NOT NULL DEFAULT 0,
    frame_count   INTEGER      NOT NULL DEFAULT 0,
    status        VARCHAR(20)  NOT NULL,
    error_message TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT ck_videos_status CHECK (status IN ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED'))
);

-- Listagem de status por usuário (consulta principal da API).
CREATE INDEX IF NOT EXISTS ix_videos_user_created ON videos (user_id, created_at DESC);

-- Transactional outbox: garante que todo upload aceito gere a mensagem de processamento.
CREATE TABLE IF NOT EXISTS outbox (
    id           UUID PRIMARY KEY,
    routing_key  VARCHAR(100) NOT NULL,
    payload      JSONB        NOT NULL,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS ix_outbox_pending ON outbox (created_at) WHERE published_at IS NULL;
