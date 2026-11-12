package bcrypt

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHasher(t *testing.T) {
	h := New(bcrypt.MinCost)
	hash, err := h.Hash("supersecret")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "supersecret" || !h.Matches(hash, "supersecret") || h.Matches(hash, "other") {
		t.Fatal("hash/match mismatch")
	}
	if New(0).cost != bcrypt.DefaultCost {
		t.Fatal("default cost")
	}
}
