// auth-service: cadastro de usuários e emissão de JWT.
// Este arquivo é a raiz de composição: cria os adaptadores (frameworks & drivers),
// injeta-os nos casos de uso e expõe os casos de uso via HTTP.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	authbcrypt "github.com/monnclaro/soat-hackaton/internal/auth/adapter/bcrypt"
	"github.com/monnclaro/soat-hackaton/internal/auth/adapter/httpapi"
	"github.com/monnclaro/soat-hackaton/internal/auth/adapter/jwt"
	authpg "github.com/monnclaro/soat-hackaton/internal/auth/adapter/postgres"
	"github.com/monnclaro/soat-hackaton/internal/auth/usecase"
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
	if err := postgres.Migrate(startCtx, pool, authpg.Migration); err != nil {
		logger.Error("falha na migração", "error", err)
		os.Exit(1)
	}

	// Adaptadores → casos de uso → controller HTTP.
	auth := usecase.NewAuth(
		authpg.NewUserRepository(pool),
		authbcrypt.New(config.Int("BCRYPT_COST", 0)),
		jwt.New(tokens),
	)
	mux := http.NewServeMux()
	httpapi.NewHandler(auth, tokens, logger).Register(mux)
	httpx.RegisterOps(mux, pool.Ping)

	if err := httpx.Serve(ctx, logger, ":"+config.String("PORT", "8081"), httpx.Observe(logger, mux)); err != nil {
		logger.Error("servidor encerrado com erro", "error", err)
		os.Exit(1)
	}
}
