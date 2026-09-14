package store

import (
	"context"
	"testing"
)

// The watermark is a row id because row ids are monotonic in the order this
// client learned about messages. A date would announce a mail that was sent
// last week but only reached us today as something new.
func TestMessagesAfterIDReturnsOnlyWhatIsNewer(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1, 2, 3)

	all, err := s.ListMessages(ctx, inbox, 50, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("seeded %d messages, want 3", len(all))
	}

	// ListMessages is newest first, so the oldest row is last.
	oldest := all[len(all)-1].ID

	got, err := s.MessagesAfterID(ctx, inbox, oldest, 50)
	if err != nil {
		t.Fatalf("MessagesAfterID() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d messages after the oldest, want 2", len(got))
	}
	for _, m := range got {
		if m.ID <= oldest {
			t.Errorf("message %d is not above the watermark %d", m.ID, oldest)
		}
	}
}

// Oldest first, because the caller advances the watermark to the last of them
// and names the last of them in the notification.
func TestMessagesAfterIDComesBackOldestFirst(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1, 2, 3)

	got, err := s.MessagesAfterID(ctx, inbox, 0, 50)
	if err != nil {
		t.Fatalf("MessagesAfterID() error: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i].ID <= got[i-1].ID {
			t.Fatalf("ids came back %d then %d, want ascending", got[i-1].ID, got[i].ID)
		}
	}
}

// A mailbox being backfilled produces one sentence, not a thousand.
func TestMessagesAfterIDIsCapped(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1, 2, 3, 4, 5)

	got, err := s.MessagesAfterID(ctx, inbox, 0, 2)
	if err != nil {
		t.Fatalf("MessagesAfterID() error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("got %d messages with a limit of 2", len(got))
	}
}

// This is where a watermark starts. Without it the first pass after startup
// would announce every unread message the mailbox already held.
func TestHighestMessageIDIsTheTopOfTheFolder(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedFolderMessages(t, s, acct, inbox, 1, 2, 3)

	highest, err := s.HighestMessageID(ctx, inbox)
	if err != nil {
		t.Fatalf("HighestMessageID() error: %v", err)
	}

	after, err := s.MessagesAfterID(ctx, inbox, highest, 50)
	if err != nil {
		t.Fatalf("MessagesAfterID() error: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("%d messages sit above the highest id", len(after))
	}
}

// An empty folder has no highest id, and that has to be zero rather than an
// error: it is the normal state of a mailbox nobody has opened.
func TestHighestMessageIDOfAnEmptyFolderIsZero(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	_, inbox := seedInbox(t, s)

	highest, err := s.HighestMessageID(ctx, inbox)
	if err != nil {
		t.Fatalf("HighestMessageID() error: %v", err)
	}
	if highest != 0 {
		t.Errorf("HighestMessageID() = %d for an empty folder", highest)
	}
}

// Arrivals in one folder must not announce mail in another: the notification
// is about the inbox.
func TestMessagesAfterIDIsScopedToOneFolder(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	archive := folderIDByPath(t, s, acct, "INBOX")
	if archive != inbox {
		t.Fatalf("precondition: %d != %d", archive, inbox)
	}
	seedFolderMessages(t, s, acct, inbox, 1, 2)

	got, err := s.MessagesAfterID(ctx, inbox+999, 0, 50)
	if err != nil {
		t.Fatalf("MessagesAfterID() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a folder with no rows returned %d messages", len(got))
	}
}
