package app

import (
	"errors"
	"testing"

	"nexusmail/internal/mailto"
)

func TestPendingMailtoIsHandedOverOnce(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})
	svc.pendingMailto = &mailto.Draft{
		To: "ali@example.com", Cc: "destek@example.com", Bcc: "gizli@example.com",
		Subject: "Konu", Body: "gövde", InReplyTo: "parent@example.com",
	}

	got, err := svc.PendingMailto()
	if err != nil {
		t.Fatalf("PendingMailto() error: %v", err)
	}
	if got.To != "ali@example.com" || got.Cc != "destek@example.com" ||
		got.Bcc != "gizli@example.com" || got.Subject != "Konu" ||
		got.Body != "gövde" || got.InReplyTo != "parent@example.com" {
		t.Errorf("got %+v", got)
	}

	again, err := svc.PendingMailto()
	if err != nil {
		t.Fatalf("second PendingMailto() error: %v", err)
	}
	if again != (MailtoDTO{}) {
		t.Errorf("the link was handed over twice: %+v", again)
	}
}

func TestPendingMailtoReportsAMalformedLink(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})
	svc.mailtoErr = errors.New("mailto: not a link")

	if _, err := svc.PendingMailto(); err == nil {
		t.Error("PendingMailto() hid the parse error")
	}
	if _, err := svc.PendingMailto(); err != nil {
		t.Errorf("the error was reported twice: %v", err)
	}
}

func TestParsePendingMailto(t *testing.T) {
	if draft, err := parsePendingMailto(""); err != nil || draft != nil {
		t.Errorf("empty link = %+v, %v; want nothing", draft, err)
	}
	if _, err := parsePendingMailto("https://example.com"); err == nil {
		t.Error("parsePendingMailto() accepted a non-mailto link")
	}
	draft, err := parsePendingMailto("mailto:ali@example.com?subject=Konu")
	if err != nil {
		t.Fatalf("parsePendingMailto() error: %v", err)
	}
	if draft.To != "ali@example.com" || draft.Subject != "Konu" {
		t.Errorf("got %+v", draft)
	}
}
