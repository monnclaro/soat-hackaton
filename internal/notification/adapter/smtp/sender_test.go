package smtp

import (
	"context"
	"net/smtp"
	"strings"
	"testing"

	"github.com/monnclaro/soat-hackaton/internal/notification/domain"
)

func TestSend(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	var gotAuth smtp.Auth
	s := New("mail:1025", "FIAP X <no-reply@fiapx.local>", "", "")
	s.send = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotAddr, gotAuth, gotFrom, gotTo, gotMsg = addr, a, from, to, msg
		return nil
	}

	if err := s.Send(context.Background(), domain.Message{To: "ana@x.com\r\nBcc: evil@x.com"}); err == nil {
		t.Fatal("header injection in recipient must be rejected")
	}
	err := s.Send(context.Background(), domain.Message{To: "Ana <ana@x.com>", Subject: "Falha no vídeo\r\nBcc: evil@x.com", Body: "linha1\nlinha2"})
	if err != nil {
		t.Fatal(err)
	}
	if gotAddr != "mail:1025" || gotAuth != nil || gotFrom != "no-reply@fiapx.local" || len(gotTo) != 1 || gotTo[0] != "ana@x.com" {
		t.Fatalf("envelope: %s %v %s %v", gotAddr, gotAuth, gotFrom, gotTo)
	}
	raw := string(gotMsg)
	if strings.Contains(raw, "\r\nBcc:") {
		t.Fatal("header injection must be prevented")
	}
	if !strings.Contains(raw, "linha1\r\nlinha2") || !strings.Contains(raw, "Subject: =?utf-8?q?") {
		t.Fatalf("raw message:\n%s", raw)
	}
}

func TestSendAuthAndInvalidFrom(t *testing.T) {
	var gotAuth smtp.Auth
	s := New("smtp.example.com:587", "a@b.com", "user", "pass")
	s.send = func(_ string, a smtp.Auth, _ string, _ []string, _ []byte) error {
		gotAuth = a
		return nil
	}
	_ = s.Send(context.Background(), domain.Message{To: "x@y.com"})
	if gotAuth == nil {
		t.Fatal("auth must be configured when username is set")
	}

	bad := New("mail:1025", "not an address", "", "")
	if err := bad.Send(context.Background(), domain.Message{To: "x@y.com"}); err == nil {
		t.Fatal("invalid SMTP_FROM must fail")
	}
}
