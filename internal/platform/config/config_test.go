package config

import (
	"testing"
	"time"
)

func TestDefaultsAndOverrides(t *testing.T) {
	if got := String("FIAPX_TEST_STR", "def"); got != "def" {
		t.Fatalf("String default = %q", got)
	}
	t.Setenv("FIAPX_TEST_STR", "value")
	if got := String("FIAPX_TEST_STR", "def"); got != "value" {
		t.Fatalf("String = %q", got)
	}

	t.Setenv("FIAPX_TEST_INT", "42")
	if got := Int("FIAPX_TEST_INT", 1); got != 42 {
		t.Fatalf("Int = %d", got)
	}
	t.Setenv("FIAPX_TEST_INT", "nope")
	if got := Int("FIAPX_TEST_INT", 1); got != 1 {
		t.Fatalf("Int invalid should fallback, got %d", got)
	}

	t.Setenv("FIAPX_TEST_I64", "5000000000")
	if got := Int64("FIAPX_TEST_I64", 1); got != 5000000000 {
		t.Fatalf("Int64 = %d", got)
	}
	if got := Int64("FIAPX_TEST_I64_MISSING", 7); got != 7 {
		t.Fatalf("Int64 default = %d", got)
	}

	t.Setenv("FIAPX_TEST_DUR", "1m30s")
	if got := Duration("FIAPX_TEST_DUR", time.Second); got != 90*time.Second {
		t.Fatalf("Duration = %v", got)
	}
	t.Setenv("FIAPX_TEST_DUR", "bad")
	if got := Duration("FIAPX_TEST_DUR", time.Second); got != time.Second {
		t.Fatalf("Duration invalid should fallback, got %v", got)
	}
}
