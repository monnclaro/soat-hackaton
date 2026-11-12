package rabbitmq

import (
	"errors"
	"fmt"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestDecide(t *testing.T) {
	if decide(nil) != actionAck {
		t.Fatal("nil should ack")
	}
	if decide(errors.New("x")) != actionRequeue {
		t.Fatal("plain error should requeue")
	}
	if decide(Permanent(errors.New("x"))) != actionReject {
		t.Fatal("permanent should reject")
	}
	wrapped := fmt.Errorf("ctx: %w", Permanent(errors.New("bad json")))
	if !IsPermanent(wrapped) || decide(wrapped) != actionReject {
		t.Fatal("wrapped permanent should reject")
	}
	if Permanent(errors.New("msg")).Error() != "msg" {
		t.Fatal("permanent should keep message")
	}
	inner := errors.New("inner")
	if !errors.Is(Permanent(inner), inner) {
		t.Fatal("permanent should unwrap")
	}
}

func TestRetryDelay(t *testing.T) {
	cases := map[int]time.Duration{0: time.Second, 1: time.Second, 2: 4 * time.Second, 3: 9 * time.Second, 10: 30 * time.Second}
	for attempt, want := range cases {
		if got := retryDelay(attempt); got != want {
			t.Errorf("retryDelay(%d) = %v, want %v", attempt, got, want)
		}
	}
}

func TestAttemptFrom(t *testing.T) {
	cases := []struct {
		h    amqp.Table
		want int
	}{
		{nil, 1},
		{amqp.Table{"x-delivery-count": int64(2)}, 3},
		{amqp.Table{"x-delivery-count": int32(1)}, 2},
		{amqp.Table{"x-delivery-count": 4}, 5},
		{amqp.Table{"x-delivery-count": "x"}, 1},
	}
	for _, c := range cases {
		if got := attemptFrom(c.h); got != c.want {
			t.Fatalf("attemptFrom(%v) = %d, want %d", c.h, got, c.want)
		}
	}
}
