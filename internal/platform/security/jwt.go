// Package security concentra emissão/validação de JWT e o contexto do usuário autenticado.
package security

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "fiapx-auth"

// ErrInvalidToken indica token ausente, expirado ou com assinatura inválida.
var ErrInvalidToken = errors.New("token inválido")

// Principal é o usuário autenticado extraído do token.
type Principal struct {
	UserID string
	Email  string
	Name   string
}

type claims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	jwt.RegisteredClaims
}

// TokenManager emite e valida tokens HS256 com segredo compartilhado entre os serviços.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewTokenManager cria um TokenManager. O segredo deve ter pelo menos 32 bytes.
func NewTokenManager(secret string, ttl time.Duration) (*TokenManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("JWT_SECRET deve ter pelo menos 32 caracteres")
	}
	return &TokenManager{secret: []byte(secret), ttl: ttl, now: time.Now}, nil
}

// Issue gera um token para o usuário.
func (m *TokenManager) Issue(p Principal) (string, time.Time, error) {
	now := m.now()
	exp := now.Add(m.ttl)
	c := claims{
		Email: p.Email,
		Name:  p.Name,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   p.UserID,
			Issuer:    issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("assinando token: %w", err)
	}
	return signed, exp, nil
}

// Verify valida o token e retorna o Principal.
func (m *TokenManager) Verify(token string) (Principal, error) {
	var c claims
	parsed, err := jwt.ParseWithClaims(token, &c, func(t *jwt.Token) (any, error) {
		return m.secret, nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil || !parsed.Valid || c.Subject == "" {
		return Principal{}, ErrInvalidToken
	}
	return Principal{UserID: c.Subject, Email: c.Email, Name: c.Name}, nil
}

type ctxKey struct{}

// WithPrincipal anexa o usuário autenticado ao contexto.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// PrincipalFrom recupera o usuário autenticado do contexto.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}
