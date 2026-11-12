package jwt

import (
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/auth/usecase"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

func TestIssuer(t *testing.T) {
	tm, _ := security.NewTokenManager("0123456789abcdef0123456789abcdef", time.Hour)
	tok, exp, err := New(tm).Issue(usecase.TokenClaims{UserID: "u1", Email: "a@b.com", Name: "Ana"})
	if err != nil || time.Until(exp) <= 0 {
		t.Fatalf("issue: %v", err)
	}
	p, err := tm.Verify(tok)
	if err != nil || p.UserID != "u1" || p.Email != "a@b.com" || p.Name != "Ana" {
		t.Fatalf("principal = %+v %v", p, err)
	}
}
