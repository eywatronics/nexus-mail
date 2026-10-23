package app

import (
	"errors"
	"testing"
)

func TestComposerWindowRequestIsHandedToTheHostAndTakenOnce(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	var opened string
	svc.cfg.OpenComposerWindow = func(id string) error {
		opened = id
		return nil
	}

	draft := DraftRecordDTO{
		AccountID: 4, To: "ali@example.com", Subject: "Konu",
		Body: "gövde", InReplyTo: "parent@example.com",
	}
	id, err := svc.OpenComposerWindow(draft)
	if err != nil {
		t.Fatalf("OpenComposerWindow() error: %v", err)
	}
	if id == "" || opened != id {
		t.Fatalf("id = %q, host saw %q", id, opened)
	}

	got, err := svc.TakeComposerWindow(id)
	if err != nil {
		t.Fatalf("TakeComposerWindow() error: %v", err)
	}
	if got.AccountID != draft.AccountID || got.To != draft.To ||
		got.Subject != draft.Subject || got.Body != draft.Body ||
		got.InReplyTo != draft.InReplyTo {
		t.Errorf("got %+v", got)
	}

	if _, err := svc.TakeComposerWindow(id); err == nil {
		t.Error("the same window request was handed over twice")
	}
}

func TestComposerWindowReportsAFailedOpen(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})
	svc.cfg.OpenComposerWindow = func(string) error { return errors.New("no window") }

	id, err := svc.OpenComposerWindow(DraftRecordDTO{AccountID: 4, To: "a@example.com"})
	if err == nil {
		t.Fatal("OpenComposerWindow() hid the host error")
	}
	if id != "" {
		t.Errorf("id = %q on failure", id)
	}
	if _, err := svc.TakeComposerWindow(id); err == nil {
		t.Error("a failed open left a request behind")
	}
}

func TestComposerWindowNeedsAnAccountAndAHost(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if _, err := svc.OpenComposerWindow(DraftRecordDTO{}); err == nil {
		t.Error("OpenComposerWindow() accepted a draft with no account")
	}

	svc.cfg.OpenComposerWindow = nil
	if _, err := svc.OpenComposerWindow(DraftRecordDTO{AccountID: 4}); err == nil {
		t.Error("OpenComposerWindow() accepted a build with no window system")
	}
}
