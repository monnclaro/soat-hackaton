package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestJobNames(t *testing.T) {
	j := Job{UserID: "u1", VideoID: "v1", OriginalName: "Aula.MP4"}
	if j.ZipKey() != "outputs/u1/v1.zip" || j.InputExtension() != ".mp4" {
		t.Fatalf("zip=%s ext=%s", j.ZipKey(), j.InputExtension())
	}
}

func TestRetryPolicy(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 3}
	transient := errors.New("storage timeout")

	if p.Decide(transient, 1) != OutcomeRetry || p.Decide(transient, 2) != OutcomeRetry {
		t.Fatal("transient errors must retry before the last attempt")
	}
	if p.Decide(transient, 3) != OutcomeFailed {
		t.Fatal("last attempt must fail")
	}
	if p.Decide(ErrNoFrames, 1) != OutcomeFailed || p.Decide(fmt.Errorf("wrap: %w", ErrTimeout), 1) != OutcomeFailed {
		t.Fatal("processing errors must fail immediately")
	}

	if got := p.FailureReason(ErrNoFrames, 1); got != ErrNoFrames.Reason {
		t.Fatal(got)
	}
	if got := p.FailureReason(transient, 3); !strings.Contains(got, "3 tentativas") || !strings.Contains(got, "storage timeout") {
		t.Fatal(got)
	}
	if ErrTimeout.Error() == "" {
		t.Fatal("error message")
	}
}
