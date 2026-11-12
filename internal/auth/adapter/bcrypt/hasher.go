// Package bcrypt implementa a porta PasswordHasher com bcrypt.
package bcrypt

import "golang.org/x/crypto/bcrypt"

// Hasher gera hashes bcrypt com o custo configurado.
type Hasher struct{ cost int }

// New cria o hasher. cost <= 0 usa o custo padrão (10).
func New(cost int) Hasher {
	if cost <= 0 {
		cost = bcrypt.DefaultCost
	}
	return Hasher{cost: cost}
}

// Hash gera o hash da senha.
func (h Hasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	return string(b), err
}

// Matches confere a senha com o hash (comparação em tempo constante).
func (h Hasher) Matches(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
