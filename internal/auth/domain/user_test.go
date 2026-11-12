package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestNewRegistrationNormalizes(t *testing.T) {
	r, err := NewRegistration("  Ana ", " Ana@Example.COM ", "supersecret")
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "Ana" || r.Email != "ana@example.com" || r.Password != "supersecret" {
		t.Fatalf("registration = %+v", r)
	}
}

func TestNewRegistrationValidation(t *testing.T) {
	cases := []struct{ name, email, pwd string }{
		{"", "a@b.com", "12345678"},
		{strings.Repeat("x", 121), "a@b.com", "12345678"},
		{"Ana", "not-an-email", "12345678"},
		{"Ana", strings.Repeat("a", 250) + "@b.com", "12345678"},
		{"Ana", "a@b.com", "short"},
		{"Ana", "a@b.com", strings.Repeat("x", 73)},
	}
	for _, c := range cases {
		_, err := NewRegistration(c.name, c.email, c.pwd)
		var ve ValidationError
		if !errors.As(err, &ve) || ve.Error() == "" {
			t.Errorf("NewRegistration(%q, %q) expected ValidationError, got %v", c.name, c.email, err)
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  X@Y.COM "); got != "x@y.com" {
		t.Fatal(got)
	}
}
