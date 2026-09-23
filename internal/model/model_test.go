package model

import "testing"

// Servers disagree about the capitalisation of system flags, so comparison
// must be case-insensitive or a message marked read on one server looks unread
// on another.
func TestMessageHasFlagIgnoresCase(t *testing.T) {
	m := Message{Flags: []string{"\\seen", "\\FLAGGED"}}

	for _, flag := range []string{FlagSeen, "\\Seen", "\\SEEN", "\\seen"} {
		if !m.HasFlag(flag) {
			t.Errorf("HasFlag(%q) = false, want true", flag)
		}
	}
	if !m.HasFlag(FlagFlagged) {
		t.Errorf("HasFlag(%q) = false, want true", FlagFlagged)
	}
	if m.HasFlag(FlagDeleted) {
		t.Errorf("HasFlag(%q) = true, want false", FlagDeleted)
	}
}

func TestMessageHasFlagOnEmptyFlags(t *testing.T) {
	var m Message
	if m.HasFlag(FlagSeen) {
		t.Error("HasFlag() on a message with no flags returned true")
	}
}

func TestFolderIsInboxIsCaseInsensitive(t *testing.T) {
	// INBOX is the one mailbox name IMAP defines as case-insensitive, and
	// servers really do return all three spellings.
	for _, path := range []string{"INBOX", "Inbox", "inbox"} {
		if !(Folder{Path: path}).IsInbox() {
			t.Errorf("IsInbox() = false for path %q, want true", path)
		}
	}
	for _, path := range []string{"Archive", "INBOX/Sub", "Inboxes"} {
		if (Folder{Path: path}).IsInbox() {
			t.Errorf("IsInbox() = true for path %q, want false", path)
		}
	}
}

func TestFolderHasAttributeIgnoresCase(t *testing.T) {
	f := Folder{Attributes: []string{"\\HasNoChildren", "\\sent"}}

	if !f.HasAttribute("\\Sent") {
		t.Error(`HasAttribute("\\Sent") = false, want true`)
	}
	if f.HasAttribute("\\Trash") {
		t.Error(`HasAttribute("\\Trash") = true, want false`)
	}
}
