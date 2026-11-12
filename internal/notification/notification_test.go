package notification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/monnclaro/soat-hackaton/internal/events"
	"github.com/monnclaro/soat-hackaton/internal/platform/rabbitmq"
)

type fakeSender struct {
	sent []Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, m Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

func failedEvent() events.VideoStatusChanged {
	return events.VideoStatusChanged{
		VideoID: "v1", UserEmail: "ana@x.com", UserName: "Ana", OriginalName: "aula.mp4",
		Status: events.StatusFailed, Error: "vídeo corrompido", OccurredAt: time.Now(),
	}
}

func newNotifier(s Sender) *Notifier {
	return NewNotifier(s, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestHandleSendsOnlyOnFailure(t *testing.T) {
	s := &fakeSender{}
	n := newNotifier(s)
	ctx := context.Background()

	ok := failedEvent()
	ok.Status = events.StatusCompleted
	if err := n.Handle(ctx, ok); err != nil || len(s.sent) != 0 {
		t.Fatalf("completed should not notify: %v %v", err, s.sent)
	}

	noEmail := failedEvent()
	noEmail.UserEmail = ""
	if err := n.Handle(ctx, noEmail); err != nil || len(s.sent) != 0 {
		t.Fatal("event without e-mail should be skipped")
	}

	if err := n.Handle(ctx, failedEvent()); err != nil {
		t.Fatal(err)
	}
	if len(s.sent) != 1 || s.sent[0].To != "ana@x.com" || !strings.Contains(s.sent[0].Body, "vídeo corrompido") {
		t.Fatalf("sent = %+v", s.sent)
	}

	s.err = errors.New("smtp down")
	if err := n.Handle(ctx, failedEvent()); err == nil {
		t.Fatal("send failure must return error (requeue)")
	}
}

func TestBuildFailureMessageDefaults(t *testing.T) {
	evt := failedEvent()
	evt.UserName, evt.Error = "", ""
	m := BuildFailureMessage(evt)
	if !strings.Contains(m.Body, "usuário") || !strings.Contains(m.Body, "erro desconhecido") || !strings.Contains(m.Subject, "aula.mp4") {
		t.Fatalf("message = %+v", m)
	}
}

func TestHandlerAdapter(t *testing.T) {
	s := &fakeSender{}
	h := Handler(newNotifier(s))
	if err := h(context.Background(), rabbitmq.Delivery{Body: []byte("x")}); !rabbitmq.IsPermanent(err) {
		t.Fatalf("invalid json should be permanent: %v", err)
	}
	body, _ := json.Marshal(failedEvent())
	if err := h(context.Background(), rabbitmq.Delivery{Body: body}); err != nil || len(s.sent) != 1 {
		t.Fatalf("handler: %v %d", err, len(s.sent))
	}
}

func TestSMTPSender(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	var gotAuth smtp.Auth
	s := NewSMTPSender("mail:1025", "FIAP X <no-reply@fiapx.local>", "", "")
	s.send = func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
		gotAddr, gotAuth, gotFrom, gotTo, gotMsg = addr, a, from, to, msg
		return nil
	}
	if err := s.Send(context.Background(), Message{To: "ana@x.com\r\nBcc: evil@x.com"}); err == nil {
		t.Fatal("header injection in recipient must be rejected")
	}
	err := s.Send(context.Background(), Message{To: "Ana <ana@x.com>", Subject: "Falha no vídeo\r\nBcc: evil@x.com", Body: "linha1\nlinha2"})
	if err != nil {
		t.Fatal(err)
	}
	// Envelope SMTP com endereços puros (sem nome de exibição).
	if gotAddr != "mail:1025" || gotAuth != nil || gotFrom != "no-reply@fiapx.local" || len(gotTo) != 1 || gotTo[0] != "ana@x.com" {
		t.Fatalf("send args: %s %v %s %v", gotAddr, gotAuth, gotFrom, gotTo)
	}
	raw := string(gotMsg)
	if strings.Contains(raw, "\r\nBcc:") {
		t.Fatal("header injection must be prevented")
	}
	if !strings.Contains(raw, "linha1\r\nlinha2") || !strings.Contains(raw, "Subject: =?utf-8?q?") {
		t.Fatalf("raw message:\n%s", raw)
	}

	auth := NewSMTPSender("smtp.example.com:587", "a@b.com", "user", "pass")
	auth.send = func(_ string, a smtp.Auth, _ string, _ []string, _ []byte) error {
		gotAuth = a
		return nil
	}
	_ = auth.Send(context.Background(), Message{To: "x@y.com"})
	if gotAuth == nil {
		t.Fatal("auth should be configured when username is set")
	}

	bad := NewSMTPSender("mail:1025", "not an address", "", "")
	if err := bad.Send(context.Background(), Message{To: "x@y.com"}); err == nil {
		t.Fatal("invalid SMTP_FROM must fail")
	}
}
