package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func addSender(t *testing.T, s *Store, accountID, folderID int64, uid int, name, addr string, at int64) {
	t.Helper()
	err := s.UpsertMessages(context.Background(), folderID, []model.Message{{
		AccountID:    accountID,
		FolderID:     folderID,
		UID:          uint32(uid),
		Subject:      "hello",
		From:         model.Address{Name: name, Addr: addr},
		InternalDate: time.Unix(at, 0),
	}})
	if err != nil {
		t.Fatalf("UpsertMessages(UID %d) error: %v", uid, err)
	}
}

func TestSuggestRecipientsFindsPeopleWhoWrote(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	accountID, folderID := seedInbox(t, s)

	addSender(t, s, accountID, folderID, 1, "Alice", "alice@example.com", 100)
	addSender(t, s, accountID, folderID, 2, "Bob", "bob@example.org", 200)

	got, err := s.SuggestRecipients(ctx, accountID, "ali", 10)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d suggestions, want 1", len(got))
	}
	if got[0].Addr != "alice@example.com" || got[0].Name != "Alice" {
		t.Errorf("got %+v, want Alice <alice@example.com>", got[0])
	}

	// The domain half of the address is searchable too: both of these people
	// are at example.com/example.org, and a typed "@example" should find them.
	got, err = s.SuggestRecipients(ctx, accountID, "example", 10)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d suggestions for a shared domain, want 2", len(got))
	}
}

// People change how they sign. One address, one suggestion, with the name from
// the message the reader saw most recently.
func TestSuggestRecipientsUsesTheNewestName(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	accountID, folderID := seedInbox(t, s)

	addSender(t, s, accountID, folderID, 1, "Alice", "alice@example.com", 100)
	addSender(t, s, accountID, folderID, 2, "Alice Smith", "ALICE@example.com", 300)

	got, err := s.SuggestRecipients(ctx, accountID, "alice", 10)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d suggestions for one address, want 1", len(got))
	}
	if got[0].Name != "Alice Smith" {
		t.Errorf("Name = %q, want the newest signature %q", got[0].Name, "Alice Smith")
	}
}

func TestSuggestRecipientsOrdersAndLimits(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	accountID, folderID := seedInbox(t, s)

	addSender(t, s, accountID, folderID, 1, "Old", "old@example.com", 100)
	addSender(t, s, accountID, folderID, 2, "New", "new@example.com", 200)
	addSender(t, s, accountID, folderID, 3, "Newest", "newest@example.com", 300)

	got, err := s.SuggestRecipients(ctx, accountID, "example", 2)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d suggestions, want the limit of 2", len(got))
	}
	if got[0].Addr != "newest@example.com" || got[1].Addr != "new@example.com" {
		t.Errorf("got %q then %q, want newest first", got[0].Addr, got[1].Addr)
	}
}

// A percent sign in a typed address is a percent sign, not "match everything".
func TestSuggestRecipientsTreatsWildcardsAsText(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	accountID, folderID := seedInbox(t, s)

	addSender(t, s, accountID, folderID, 1, "Alice", "alice@example.com", 100)
	addSender(t, s, accountID, folderID, 2, "Percent", "percent@example.com", 200)

	got, err := s.SuggestRecipients(ctx, accountID, "%", 10)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a bare %% matched %d addresses; it should match only a literal %%", len(got))
	}
}

func TestSuggestRecipientsIgnoresEmptyInputAndAddresses(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	accountID, folderID := seedInbox(t, s)

	addSender(t, s, accountID, folderID, 1, "No Address", "", 100)

	got, err := s.SuggestRecipients(ctx, accountID, "anything", 10)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d suggestions for a message with no From address, want 0", len(got))
	}

	got, err = s.SuggestRecipients(ctx, accountID, "  ", 10)
	if err != nil {
		t.Fatalf("SuggestRecipients() error: %v", err)
	}
	if got != nil {
		t.Errorf("SuggestRecipients() with blank input = %v, want nil", got)
	}
}
