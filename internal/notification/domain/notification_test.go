package domain

import (
	"strings"
	"testing"
	"time"
)

func TestFailureMessage(t *testing.T) {
	f := Failure{VideoID: "v1", UserEmail: "ana@x.com", UserName: "Ana", OriginalName: "aula.mp4", Reason: "vídeo corrompido", OccurredAt: time.Now()}
	m := FailureMessage(f)
	if m.To != "ana@x.com" || !strings.Contains(m.Subject, "aula.mp4") || !strings.Contains(m.Body, "Olá, Ana") || !strings.Contains(m.Body, "vídeo corrompido") {
		t.Fatalf("message = %+v", m)
	}

	f.UserName, f.Reason = "", ""
	m = FailureMessage(f)
	if !strings.Contains(m.Body, "usuário") || !strings.Contains(m.Body, "erro desconhecido") {
		t.Fatalf("defaults = %+v", m)
	}
}

func TestNotifiable(t *testing.T) {
	if (Failure{}).Notifiable() || !(Failure{UserEmail: "a@b.com"}).Notifiable() {
		t.Fatal("notifiable")
	}
}
