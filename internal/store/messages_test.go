package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func seedInbox(t *testing.T, s *Store) (accountID, folderID int64) {
	t.Helper()
	ctx := context.Background()
	accountID = seedAccount(t, s)
	if err := s.UpsertFolders(ctx, accountID, []model.Folder{{Path: "INBOX", Name: "INBOX"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	return accountID, folderIDByPath(t, s, accountID, "INBOX")
}

func TestUpsertMessagesRoundTripsEveryField(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	want := model.Message{
		AccountID: acct, FolderID: fid, UID: 42,
		MessageID:      "<a@example.com>",
		ThreadID:       "<root@example.com>",
		InReplyTo:      "<b@example.com>",
		References:     []string{"<root@example.com>", "<b@example.com>"},
		Subject:        "Invoice",
		From:           model.Address{Name: "Ali", Addr: "ali@example.com"},
		To:             []model.Address{{Name: "Me", Addr: "me@example.com"}},
		Cc:             []model.Address{{Addr: "cc@example.com"}},
		Date:           time.Unix(1700000000, 0),
		InternalDate:   time.Unix(1700000001, 0),
		Size:           2048,
		Snippet:        "Please find attached",
		Flags:          []string{model.FlagSeen},
		HasAttachments: true,
	}

	if err := s.UpsertMessages(ctx, fid, []model.Message{want}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	got, err := s.ListMessages(ctx, fid, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListMessages() returned %d messages, want 1", len(got))
	}

	m := got[0]
	if m.UID != want.UID {
		t.Errorf("UID = %d, want %d", m.UID, want.UID)
	}
	if m.MessageID != want.MessageID {
		t.Errorf("MessageID = %q, want %q", m.MessageID, want.MessageID)
	}
	if m.ThreadID != want.ThreadID {
		t.Errorf("ThreadID = %q, want %q", m.ThreadID, want.ThreadID)
	}
	if len(m.References) != 2 || m.References[0] != "<root@example.com>" {
		t.Errorf("References = %v, want the two-entry ancestry", m.References)
	}
	if m.Subject != want.Subject {
		t.Errorf("Subject = %q, want %q", m.Subject, want.Subject)
	}
	if m.From != want.From {
		t.Errorf("From = %+v, want %+v", m.From, want.From)
	}
	if len(m.To) != 1 || m.To[0].Addr != "me@example.com" {
		t.Errorf("To = %+v, want one address me@example.com", m.To)
	}
	if len(m.Cc) != 1 || m.Cc[0].Addr != "cc@example.com" {
		t.Errorf("Cc = %+v, want one address cc@example.com", m.Cc)
	}
	if !m.Date.Equal(want.Date) {
		t.Errorf("Date = %v, want %v", m.Date, want.Date)
	}
	if !m.InternalDate.Equal(want.InternalDate) {
		t.Errorf("InternalDate = %v, want %v", m.InternalDate, want.InternalDate)
	}
	if m.Size != want.Size {
		t.Errorf("Size = %d, want %d", m.Size, want.Size)
	}
	if m.Snippet != want.Snippet {
		t.Errorf("Snippet = %q, want %q", m.Snippet, want.Snippet)
	}
	if !m.HasFlag(model.FlagSeen) {
		t.Errorf("Flags = %v, want them to include \\Seen", m.Flags)
	}
	if !m.HasAttachments {
		t.Error("HasAttachments = false, want true")
	}
	if m.BodyFetched {
		t.Error("BodyFetched = true, want false — headers must not imply a body")
	}
}

func TestUpsertMessagesDeduplicatesByUID(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	msgs := []model.Message{{
		AccountID: acct, FolderID: fid, UID: 42,
		Subject: "Invoice", Flags: []string{model.FlagSeen},
		InternalDate: time.Unix(1, 0),
	}}
	if err := s.UpsertMessages(ctx, fid, msgs); err != nil {
		t.Fatalf("first UpsertMessages() error: %v", err)
	}

	// Writing the same UID again must update, not duplicate. This is what
	// keeps a reconnect from doubling the mailbox.
	msgs[0].Flags = []string{model.FlagSeen, model.FlagFlagged}
	msgs[0].Subject = "Receipt"
	if err := s.UpsertMessages(ctx, fid, msgs); err != nil {
		t.Fatalf("second UpsertMessages() error: %v", err)
	}

	got, err := s.ListMessages(ctx, fid, 100, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListMessages() returned %d messages, want 1", len(got))
	}
	if got[0].Subject != "Receipt" {
		t.Errorf("Subject = %q, want the updated Receipt", got[0].Subject)
	}
	if !got[0].HasFlag(model.FlagFlagged) {
		t.Errorf("Flags = %v, want the second write's \\Flagged", got[0].Flags)
	}
}

func TestUpsertMessagesAcceptsAnEmptyBatch(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	_, fid := seedInbox(t, s)

	// An empty folder produces an empty batch on every sync; that must not be
	// an error or open a pointless transaction.
	if err := s.UpsertMessages(ctx, fid, nil); err != nil {
		t.Errorf("UpsertMessages() with no messages returned %v, want nil", err)
	}
}

func TestListMessagesOrdersNewestFirstAndPaginates(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	var batch []model.Message
	for i := 1; i <= 5; i++ {
		batch = append(batch, model.Message{
			AccountID: acct, FolderID: fid, UID: uint32(i),
			Subject:      "message",
			InternalDate: time.Unix(int64(1700000000+i), 0),
		})
	}
	if err := s.UpsertMessages(ctx, fid, batch); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	page, err := s.ListMessages(ctx, fid, 2, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("page length = %d, want 2", len(page))
	}
	if page[0].UID != 5 || page[1].UID != 4 {
		t.Errorf("first page UIDs = %d,%d; want 5,4 (newest first)", page[0].UID, page[1].UID)
	}

	next, err := s.ListMessages(ctx, fid, 2, 2)
	if err != nil {
		t.Fatalf("ListMessages(offset) error: %v", err)
	}
	if len(next) != 2 || next[0].UID != 3 {
		t.Errorf("second page = %v, want it to start at UID 3", next)
	}

	// Ordering deliberately uses internal_date rather than the Date: header,
	// which is attacker-controlled and often wrong.
	tail, err := s.ListMessages(ctx, fid, 10, 4)
	if err != nil {
		t.Fatalf("ListMessages(tail) error: %v", err)
	}
	if len(tail) != 1 || tail[0].UID != 1 {
		t.Errorf("tail = %v, want the single oldest message UID 1", tail)
	}
}

func TestGetAndSetMessageBody(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, fid, []model.Message{
		{AccountID: acct, FolderID: fid, UID: 1, Subject: "x", InternalDate: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	msgs, _ := s.ListMessages(ctx, fid, 10, 0)
	id := msgs[0].ID

	// A message with no cached body must report empty rather than erroring:
	// that is the normal state for every message until it is first opened.
	html, text, err := s.GetMessageBody(ctx, id)
	if err != nil {
		t.Fatalf("GetMessageBody() before caching returned %v, want no error", err)
	}
	if html != "" || text != "" {
		t.Errorf("GetMessageBody() = (%q, %q), want empty strings", html, text)
	}

	if err := s.SetMessageBody(ctx, id, "<p>hi</p>", "hi"); err != nil {
		t.Fatalf("SetMessageBody() error: %v", err)
	}

	html, text, err = s.GetMessageBody(ctx, id)
	if err != nil {
		t.Fatalf("GetMessageBody() error: %v", err)
	}
	if html != "<p>hi</p>" || text != "hi" {
		t.Errorf("GetMessageBody() = (%q, %q), want (<p>hi</p>, hi)", html, text)
	}

	// The list must be able to tell what is cached without reading bodies.
	msgs, _ = s.ListMessages(ctx, fid, 10, 0)
	if !msgs[0].BodyFetched {
		t.Error("BodyFetched = false after SetMessageBody")
	}

	// Re-fetching the same message overwrites rather than failing on the
	// primary key; a body can legitimately be fetched twice.
	if err := s.SetMessageBody(ctx, id, "<p>updated</p>", "updated"); err != nil {
		t.Fatalf("second SetMessageBody() error: %v", err)
	}
	html, _, _ = s.GetMessageBody(ctx, id)
	if html != "<p>updated</p>" {
		t.Errorf("GetMessageBody() = %q after the second write, want <p>updated</p>", html)
	}
}
