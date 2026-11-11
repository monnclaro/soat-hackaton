// auth-service: cadastro de usuários e emissão de JWT.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/auth"
	"github.com/monnclaro/soat-hackaton/internal/platform/config"
	"github.com/monnclaro/soat-hackaton/internal/platform/httpx"
	"github.com/monnclaro/soat-hackaton/internal/platform/logging"
	"github.com/monnclaro/soat-hackaton/internal/platform/postgres"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

func main() {
	logger := logging.New("auth-service", config.String("LOG_LEVEL", "info"))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tokens, err := security.NewTokenManager(config.String("JWT_SECRET", ""), config.Duration("JWT_TTL", 24*time.Hour))
	if err != nil {
		logger.Error("configuração inválida", "error", err)
		os.Exit(1)
	}

	startCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	pool, err := postgres.Connect(startCtx, config.String("DATABASE_URL", "postgres://fiapx:fiapx@localhost:5432/fiapx_auth?sslmode=disable"), logger)
	if err != nil {
		logger.Error("falha ao conectar no banco", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := postgres.Migrate(startCtx, pool, auth.Migration); err != nil {
		logger.Error("falha na migração", "error", err)
		os.Exit(1)
	}

	svc := auth.NewService(auth.NewPostgresRepository(pool), tokens, config.Int("BCRYPT_COST", 0))
	mux := http.NewServeMux()
	auth.NewHandler(svc, tokens, logger).Register(mux)
	httpx.RegisterOps(mux, pool.Ping)

	if err := httpx.Serve(ctx, logger, ":"+config.String("PORT", "8081"), httpx.Observe(logger, mux)); err != nil {
		logger.Error("servidor encerrado com erro", "error", err)
		os.Exit(1)
	}
}
