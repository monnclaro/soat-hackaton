// Package jwt adapta o TokenManager da plataforma à porta TokenIssuer do caso de uso.
package jwt

import (
	"time"

	"github.com/monnclaro/soat-hackaton/internal/auth/usecase"
	"github.com/monnclaro/soat-hackaton/internal/platform/security"
)

// Issuer emite JWTs HS256.
type Issuer struct{ tm *security.TokenManager }

// New cria o emissor.
func New(tm *security.TokenManager) Issuer { return Issuer{tm: tm} }

// Issue emite o token com os dados do usuário.
func (i Issuer) Issue(c usecase.TokenClaims) (string, time.Time, error) {
	return i.tm.Issue(security.Principal{UserID: c.UserID, Email: c.Email, Name: c.Name})
}
