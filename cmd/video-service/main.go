// video-service: upload, listagem de status e download dos vídeos; publica jobs via outbox
// e consome os eventos de status do processor-worker.
// Este arquivo é a raiz de composição: cria os adaptadores e os injeta nos casos de uso.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/monnclaro/soat-hackaton/internal/platform/config"
	"github.com/monnclaro/soat-hackaton/internal/platform/httpx"
	"github.com/monnclaro/soat-hackaton/internal/platform/logging"
	"github.com/monnclaro/soat-hackaton/internal/platform/postgres"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
	"github.com/monnclaro/soat-hackaton/internal/platform/storage"
	"github.com/monnclaro/soat-hackaton/internal/video/adapter/httpapi"
	"github.com/monnclaro/soat-hackaton/internal/video/adapter/messaging"
	videopg "github.com/monnclaro/soat-hackaton/internal/video/adapter/postgres"
	videoredis "github.com/monnclaro/soat-hackaton/internal/video/adapter/redis"
	"github.com/monnclaro/soat-hackaton/internal/video/usecase"
)

func main() {
	log := logging.New("video-service", config.String("LOG_LEVEL", "info"))
	if err := run(log); err != nil {
		log.Error("video-service encerrado com erro", "error", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tokens, err := security.NewTokenManager(config.String("JWT_SECRET", ""), config.Duration("JWT_TTL", 24*time.Hour))
	if err != nil {
		return err
	}

	startCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	pool, err := postgres.Connect(startCtx, config.String("DATABASE_URL", "postgres://fiapx:fiapx@localhost:5432/fiapx_videos?sslmode=disable"), log)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := postgres.Migrate(startCtx, pool, videopg.Migration); err != nil {
		return err
	}

	rdb := goredis.NewClient(&goredis.Options{Addr: config.String("REDIS_ADDR", "localhost:6379"), Password: config.String("REDIS_PASSWORD", "")})
	defer func() { _ = rdb.Close() }()

	st, err := storage.New(startCtx, storage.Config{
		Endpoint:  config.String("S3_ENDPOINT", "localhost:8333"),
		AccessKey: config.String("S3_ACCESS_KEY", "fiapx-access-key"),
		SecretKey: config.String("S3_SECRET_KEY", "fiapx-secret-key"),
		Bucket:    config.String("S3_BUCKET", "fiapx-videos"),
		UseSSL:    config.String("S3_USE_SSL", "false") == "true",
		Region:    config.String("S3_REGION", "us-east-1"),
	}, log)
	if err != nil {
		return err
	}

	conn, err := rabbitmq.Dial(startCtx, config.String("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"), log)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if err := rabbitmq.Setup(conn, config.Int("RABBITMQ_DELIVERY_LIMIT", 5)); err != nil {
		return err
	}
	pub, err := rabbitmq.NewPublisher(conn)
	if err != nil {
		return err
	}
	defer func() { _ = pub.Close() }()

	// Adaptadores → casos de uso → controller HTTP / consumidor / relay.
	repo := videopg.NewRepository(pool)
	videos := usecase.NewVideos(repo, videoredis.NewListCache(rdb, config.Duration("CACHE_TTL", 30*time.Second)), st, log)

	mux := http.NewServeMux()
	httpapi.NewHandler(videos, tokens, log, config.Int64("MAX_UPLOAD_BYTES", 1<<30)).Register(mux)
	// Readiness só depende do que é essencial (Postgres e storage). O Redis é apenas cache:
	// se cair, a listagem lê direto do Postgres, então não deve tirar o pod do balanceamento.
	httpx.RegisterOps(mux, func(ctx context.Context) error {
		return errors.Join(repo.Ping(ctx), st.Ping(ctx))
	})

	// Erros fatais de qualquer componente encerram o processo (o orquestrador reinicia).
	ctx, cancelAll := context.WithCancelCause(ctx)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		messaging.NewOutboxRelay(repo, pub, config.Duration("OUTBOX_INTERVAL", time.Second), config.Int("OUTBOX_BATCH", 100), log).Run(ctx)
	}()
	go func() {
		defer wg.Done()
		if err := rabbitmq.Consume(ctx, conn, rabbitmq.QueueStatusChanged, config.Int("CONSUMER_CONCURRENCY", 4), log, messaging.StatusHandler(videos)); err != nil {
			cancelAll(err)
		}
	}()
	go func() {
		defer wg.Done()
		if err := httpx.Serve(ctx, log, ":"+config.String("PORT", "8082"), httpx.Observe(log, mux)); err != nil {
			cancelAll(err)
		}
	}()
	<-ctx.Done()
	wg.Wait()
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	log.Info("video-service finalizado")
	return nil
}
