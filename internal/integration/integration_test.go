//go:build integration

// Testes de integração dos adaptadores contra infraestrutura real (Postgres, Redis, RabbitMQ, S3).
// Execute com: docker compose up -d postgres redis rabbitmq s3 && go test -tags=integration ./internal/integration/...
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/monnclaro/soat-hackaton/internal/auth"
	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/postgres"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
	"github.com/monnclaro/soat-hackaton/internal/platform/storage"
	"github.com/monnclaro/soat-hackaton/internal/video"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var logger = slog.New(slog.NewTextHandler(io.Discard, nil))

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	return c
}

func pool(t *testing.T, db string, migration string) *pgxpool.Pool {
	t.Helper()
	url := env("TEST_PG_BASE", "postgres://fiapx:fiapx@localhost:5432/") + db + "?sslmode=disable"
	p, err := postgres.Connect(ctx(t), url, logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	if err := postgres.Migrate(ctx(t), p, migration); err != nil {
		t.Fatal(err)
	}
	// Migração é idempotente.
	if err := postgres.Migrate(ctx(t), p, migration); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAuthPostgresRepository(t *testing.T) {
	repo := auth.NewPostgresRepository(pool(t, "fiapx_auth", auth.Migration))
	c := ctx(t)
	email := fmt.Sprintf("it-%s@fiapx.local", uuid.NewString()[:8])
	u := auth.User{ID: uuid.NewString(), Name: "IT", Email: email, PasswordHash: "hash", CreatedAt: time.Now().UTC()}
	if err := repo.Create(c, u); err != nil {
		t.Fatal(err)
	}
	dup := u
	dup.ID = uuid.NewString()
	if err := repo.Create(c, dup); !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("duplicate e-mail: %v", err)
	}
	got, err := repo.FindByEmail(c, email)
	if err != nil || got.ID != u.ID {
		t.Fatalf("FindByEmail: %+v %v", got, err)
	}
	if _, err := repo.FindByID(c, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.FindByEmail(c, "none@x.com"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("not found: %v", err)
	}
}

func TestVideoPostgresRepositoryAndOutbox(t *testing.T) {
	p := pool(t, "fiapx_videos", video.Migration)
	repo := video.NewPostgresRepository(p)
	c := ctx(t)
	// Isola o teste: drena mensagens pendentes de execuções anteriores.
	_, _ = p.Exec(c, `UPDATE outbox SET published_at = now() WHERE published_at IS NULL`)

	userID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Millisecond)
	v := video.Video{ID: uuid.NewString(), UserID: userID, UserEmail: "a@x.com", OriginalName: "a.mp4",
		StorageKey: "videos/a.mp4", SizeBytes: 10, Status: events.StatusPending, CreatedAt: now, UpdatedAt: now}
	msg := video.OutboxMessage{ID: uuid.NewString(), RoutingKey: events.RoutingKeyVideoUploaded, Payload: []byte(`{"video_id":"x"}`), CreatedAt: now}
	if err := repo.CreateWithOutbox(c, v, msg); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(c, v.ID)
	if err != nil || got.Status != events.StatusPending || got.ZipKey != "" {
		t.Fatalf("Get: %+v %v", got, err)
	}
	if _, err := repo.Get(c, "not-a-uuid"); !errors.Is(err, video.ErrNotFound) {
		t.Fatalf("invalid id: %v", err)
	}
	if _, err := repo.Get(c, uuid.NewString()); !errors.Is(err, video.ErrNotFound) {
		t.Fatalf("missing id: %v", err)
	}

	got.Status, got.ZipKey, got.FrameCount, got.UpdatedAt = events.StatusCompleted, "outputs/z.zip", 7, time.Now().UTC()
	if err := repo.Update(c, got); err != nil {
		t.Fatal(err)
	}
	list, err := repo.ListByUser(c, userID)
	if err != nil || len(list) != 1 || list[0].FrameCount != 7 || list[0].ZipKey != "outputs/z.zip" {
		t.Fatalf("ListByUser: %+v %v", list, err)
	}
	missing := got
	missing.ID = uuid.NewString()
	if err := repo.Update(c, missing); !errors.Is(err, video.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}

	// Falha na publicação: a mensagem continua pendente.
	n, err := repo.DispatchPending(c, 10, func(context.Context, video.OutboxMessage) error { return errors.New("broker down") })
	if n != 0 || err == nil {
		t.Fatalf("failed dispatch: %d %v", n, err)
	}
	var published []string
	n, err = repo.DispatchPending(c, 10, func(_ context.Context, m video.OutboxMessage) error {
		published = append(published, m.ID)
		return nil
	})
	if err != nil || n != 1 || published[0] != msg.ID {
		t.Fatalf("dispatch: %d %v %v", n, err, published)
	}
	n, _ = repo.DispatchPending(c, 10, func(context.Context, video.OutboxMessage) error { return nil })
	if n != 0 {
		t.Fatalf("message must be published only once, got %d", n)
	}
}

func TestRedisCache(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: env("TEST_REDIS_ADDR", "localhost:6379")})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := video.NewRedisCache(rdb, time.Minute)
	c := ctx(t)
	user := uuid.NewString()

	if _, ok, err := cache.GetList(c, user); ok || err != nil {
		t.Fatalf("empty cache: %v %v", ok, err)
	}
	if err := cache.SetList(c, user, []video.Video{{ID: "v1", Status: events.StatusPending}}); err != nil {
		t.Fatal(err)
	}
	list, ok, err := cache.GetList(c, user)
	if err != nil || !ok || len(list) != 1 || list[0].ID != "v1" {
		t.Fatalf("cached: %+v %v %v", list, ok, err)
	}
	if err := cache.Invalidate(c, user); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := cache.GetList(c, user); ok {
		t.Fatal("cache should be invalidated")
	}
}

func newStorage(t *testing.T) *storage.S3 {
	t.Helper()
	st, err := storage.New(ctx(t), storage.Config{
		Endpoint: env("TEST_S3_ENDPOINT", "localhost:8333"), AccessKey: "fiapx-access-key", SecretKey: "fiapx-secret-key",
		Bucket: "fiapx-it", Region: "us-east-1",
	}, logger)
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestS3Storage(t *testing.T) {
	st := newStorage(t)
	c := ctx(t)
	key := "it/" + uuid.NewString()
	n, err := st.Put(c, key, strings.NewReader("hello"), -1, "text/plain")
	if err != nil || n != 5 {
		t.Fatalf("put: %d %v", n, err)
	}
	rc, size, err := st.Get(c, key)
	if err != nil || size != 5 {
		t.Fatalf("get: %d %v", size, err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(b) != "hello" {
		t.Fatalf("content = %q", b)
	}
	if err := st.Delete(c, key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.Get(c, key); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("deleted object: %v", err)
	}
	if err := st.Ping(c); err != nil {
		t.Fatal(err)
	}
}

// TestRabbitMQRoundTrip valida publish com confirmação, consumo, requeue de erro transitório e DLQ.
func TestRabbitMQRoundTrip(t *testing.T) {
	c := ctx(t)
	conn, err := rabbitmq.Dial(c, env("TEST_RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"), logger)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := rabbitmq.Setup(conn, 5); err != nil {
		t.Fatal(err)
	}
	ch, _ := conn.Channel()
	_, _ = ch.QueuePurge(rabbitmq.QueueNotifications, false)
	_ = ch.Close()

	pub, err := rabbitmq.NewPublisher(conn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pub.Close() })

	id := uuid.NewString()
	body, _ := json.Marshal(events.VideoStatusChanged{VideoID: id, Status: events.StatusFailed})
	if err := pub.Publish(c, events.StatusRoutingKey(events.StatusFailed), body, id); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	got := make(chan rabbitmq.Delivery, 1)
	consumeCtx, stop := context.WithCancel(c)
	done := make(chan error, 1)
	go func() {
		done <- rabbitmq.Consume(consumeCtx, conn, rabbitmq.QueueNotifications, 1, logger, func(_ context.Context, d rabbitmq.Delivery) error {
			if calls.Add(1) == 1 {
				return errors.New("transient") // primeira tentativa falha → reentrega
			}
			got <- d
			return nil
		})
	}()
	select {
	case d := <-got:
		if d.MessageID != id || !bytes.Equal(d.Body, body) || d.Attempt < 2 {
			t.Fatalf("delivery = %+v", d)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("message not redelivered")
	}
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
