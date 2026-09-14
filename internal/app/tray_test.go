package app

import (
	"context"
	"strings"
	"testing"

	"nexusmail/internal/model"
)

// The window can be closed while the app keeps running, so the tray icon is
// the only thing on screen saying whether anything arrived.
func TestUnreadCountSumsTheInboxes(t *testing.T) {
	svc, _, db := newTestService(t, stubBackend{})
	ctx := context.Background()

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	if err := db.UpsertFolders(ctx, acct.ID, []model.Folder{
		{Path: "INBOX", Name: "INBOX", UnreadCount: 4},
		{Path: "Projeler", Name: "Projeler", UnreadCount: 11},
	}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}

	got, err := svc.UnreadCount()
	if err != nil {
		t.Fatalf("UnreadCount() error: %v", err)
	}
	// A server-side rule that filed mail into a project folder has already
	// decided it is not urgent; counting it would make the badge a number
	// nobody acts on.
	if got != 4 {
		t.Errorf("UnreadCount() = %d, want only the inbox's 4", got)
	}
}

func TestUnreadCountSpansAccounts(t *testing.T) {
	svc, _, db := newTestService(t, stubBackend{})
	ctx := context.Background()

	for i, email := range []string{"a@example.com", "b@example.com"} {
		acct, err := svc.AddPasswordAccount(email, "U", "h", 993, "tls", "", 0, "pw")
		if err != nil {
			t.Fatalf("AddPasswordAccount() error: %v", err)
		}
		if err := svc.SyncAccount(acct.ID); err != nil {
			t.Fatalf("SyncAccount() error: %v", err)
		}
		if err := db.UpsertFolders(ctx, acct.ID, []model.Folder{
			{Path: "INBOX", Name: "INBOX", UnreadCount: i + 1},
		}); err != nil {
			t.Fatalf("UpsertFolders() error: %v", err)
		}
	}

	got, err := svc.UnreadCount()
	if err != nil {
		t.Fatalf("UnreadCount() error: %v", err)
	}
	if got != 3 {
		t.Errorf("UnreadCount() = %d, want 1 + 2 across both accounts", got)
	}
}

func TestUnreadCountWithNoAccounts(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	got, err := svc.UnreadCount()
	if err != nil {
		t.Fatalf("UnreadCount() error: %v", err)
	}
	if got != 0 {
		t.Errorf("UnreadCount() = %d before any account exists", got)
	}
}

// The tooltip is the only place the number can go: a count drawn into a
// 16-pixel tray glyph is unreadable at two digits and absent at three.
func TestTheTrayTooltipReadsAsASentence(t *testing.T) {
	cases := []struct {
		unread int
		want   string
	}{
		{0, "Nexus Mail"},
		{-1, "Nexus Mail"},
		{1, "Nexus Mail — 1 unread"},
		{12, "Nexus Mail — 12 unread"},
	}

	for _, c := range cases {
		if got := TrayTooltip(c.unread); got != c.want {
			t.Errorf("TrayTooltip(%d) = %q, want %q", c.unread, got, c.want)
		}
	}
}

// An empty inbox must not say "0 unread". The tray is glanced at, not read,
// and a zero there is noise that trains people to ignore the icon.
func TestTheTooltipSaysNothingWhenThereIsNothing(t *testing.T) {
	if strings.Contains(TrayTooltip(0), "0") {
		t.Errorf("TrayTooltip(0) = %q, want no count at all", TrayTooltip(0))
	}
}
