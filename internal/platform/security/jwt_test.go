package security

import (
	"context"
	"testing"
	"time"
)

const testSecret = "0123456789abcdef0123456789abcdef"

func TestNewTokenManagerRejectsShortSecret(t *testing.T) {
	if _, err := NewTokenManager("short", time.Hour); err == nil {
		t.Fatal("expected error for short secret")
	}
}

func TestIssueAndVerify(t *testing.T) {
	m, err := NewTokenManager(testSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	tok, exp, err := m.Issue(Principal{UserID: "u1", Email: "a@b.com", Name: "Ana"})
	if err != nil {
		t.Fatal(err)
	}
	if time.Until(exp) <= 0 {
		t.Fatal("expiration should be in the future")
	}
	p, err := m.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if p.UserID != "u1" || p.Email != "a@b.com" || p.Name != "Ana" {
		t.Fatalf("unexpected principal %+v", p)
	}
}

func TestVerifyRejectsTamperedExpiredAndForeign(t *testing.T) {
	m, _ := NewTokenManager(testSecret, time.Hour)
	tok, _, _ := m.Issue(Principal{UserID: "u1"})

	if _, err := m.Verify(tok + "x"); err != ErrInvalidToken {
		t.Fatalf("tampered: got %v", err)
	}

	other, _ := NewTokenManager("ffffffffffffffffffffffffffffffff", time.Hour)
	if _, err := other.Verify(tok); err != ErrInvalidToken {
		t.Fatalf("foreign secret: got %v", err)
	}

	expired, _ := NewTokenManager(testSecret, time.Minute)
	expired.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }
	old, _, _ := expired.Issue(Principal{UserID: "u1"})
	if _, err := m.Verify(old); err != ErrInvalidToken {
		t.Fatalf("expired: got %v", err)
	}
}

func TestPrincipalContext(t *testing.T) {
	if _, ok := PrincipalFrom(context.Background()); ok {
		t.Fatal("empty context should not have principal")
	}
	ctx := WithPrincipal(context.Background(), Principal{UserID: "u"})
	p, ok := PrincipalFrom(ctx)
	if !ok || p.UserID != "u" {
		t.Fatalf("got %+v %v", p, ok)
	}
}
