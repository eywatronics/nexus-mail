package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func TestInsertAndListAccounts(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	want := model.Account{
		Email:       "user@example.com",
		DisplayName: "Test User",
		Provider:    model.ProviderGeneric,
		AuthKind:    model.AuthPassword,
		IMAPHost:    "imap.example.com",
		IMAPPort:    993,
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
		SecretRef:   "account:user@example.com",
		CreatedAt:   time.Unix(1700000000, 0),
	}

	id, err := s.InsertAccount(ctx, want)
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}
	if id == 0 {
		t.Fatal("InsertAccount() returned id 0")
	}

	got, err := s.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListAccounts() returned %d accounts, want 1", len(got))
	}

	a := got[0]
	if a.ID != id {
		t.Errorf("ID = %d, want %d", a.ID, id)
	}
	if a.Email != want.Email {
		t.Errorf("Email = %q, want %q", a.Email, want.Email)
	}
	if a.DisplayName != want.DisplayName {
		t.Errorf("DisplayName = %q, want %q", a.DisplayName, want.DisplayName)
	}
	if a.Provider != want.Provider {
		t.Errorf("Provider = %q, want %q", a.Provider, want.Provider)
	}
	if a.AuthKind != want.AuthKind {
		t.Errorf("AuthKind = %q, want %q", a.AuthKind, want.AuthKind)
	}
	if a.IMAPHost != want.IMAPHost || a.IMAPPort != want.IMAPPort {
		t.Errorf("IMAP = %s:%d, want %s:%d", a.IMAPHost, a.IMAPPort, want.IMAPHost, want.IMAPPort)
	}
	if a.SMTPHost != want.SMTPHost || a.SMTPPort != want.SMTPPort {
		t.Errorf("SMTP = %s:%d, want %s:%d", a.SMTPHost, a.SMTPPort, want.SMTPHost, want.SMTPPort)
	}
	if a.SecretRef != want.SecretRef {
		t.Errorf("SecretRef = %q, want %q", a.SecretRef, want.SecretRef)
	}
	if !a.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", a.CreatedAt, want.CreatedAt)
	}
}

func TestInsertAccountRejectsDuplicateEmail(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	a := model.Account{
		Email: "dup@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "r", CreatedAt: time.Unix(1, 0),
	}
	if _, err := s.InsertAccount(ctx, a); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	// The UNIQUE constraint, not application logic, is what stops the same
	// mailbox being added twice.
	if _, err := s.InsertAccount(ctx, a); err == nil {
		t.Error("second insert with the same email succeeded, want a UNIQUE violation")
	}
}

func TestGetAccountByID(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	id, err := s.InsertAccount(ctx, model.Account{
		Email: "one@example.com", Provider: model.ProviderGoogle,
		AuthKind: model.AuthOAuth, IMAPHost: "imap.gmail.com", IMAPPort: 993,
		SecretRef: "account:one@example.com", CreatedAt: time.Unix(5, 0),
	})
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}

	got, err := s.GetAccount(ctx, id)
	if err != nil {
		t.Fatalf("GetAccount() error: %v", err)
	}
	if got.Email != "one@example.com" {
		t.Errorf("Email = %q, want one@example.com", got.Email)
	}

	// Callers resolve accounts by id constantly; a missing one must be an
	// error rather than a zero-valued account that silently does nothing.
	if _, err := s.GetAccount(ctx, id+999); err == nil {
		t.Error("GetAccount() with an unknown id returned no error")
	}
}
