package model

import "testing"

// The server stating the answer beats this code inferring it. A mailbox
// carrying \Sent is the sent folder whatever it happens to be called — which
// on a Turkish account is the whole point, because it is not called Sent.
func TestSpecialUseAttributeDecidesTheRole(t *testing.T) {
	cases := []struct {
		attr string
		want FolderRole
	}{
		{"\\Sent", RoleSent},
		{"\\Drafts", RoleDrafts},
		{"\\Trash", RoleTrash},
		{"\\Junk", RoleJunk},
		{"\\Archive", RoleArchive},
	}

	for _, c := range cases {
		f := Folder{Name: "Bir Klasör", Path: "Bir Klasör", Attributes: []string{c.attr}}
		if got := f.Role(); got != c.want {
			t.Errorf("a folder carrying %s has role %q, want %q", c.attr, got, c.want)
		}
	}
}

// Servers are inconsistent about the case of flags, the same reason
// Message.HasFlag compares case-insensitively.
func TestSpecialUseMatchingIgnoresCase(t *testing.T) {
	f := Folder{Name: "x", Path: "x", Attributes: []string{"\\sent"}}
	if got := f.Role(); got != RoleSent {
		t.Errorf("Role() = %q for a lowercase attribute, want %q", got, RoleSent)
	}
}

// Gmail's \All and \Flagged name virtual mailboxes: views over mail that lives
// in another folder. Treating one as a real folder is how a client syncs every
// message twice.
func TestGmailVirtualMailboxesGetNoRole(t *testing.T) {
	for _, attr := range []string{"\\All", "\\Flagged"} {
		f := Folder{Name: "All Mail", Path: "[Gmail]/All Mail", Attributes: []string{attr}}
		if got := f.Role(); got != RoleNone {
			t.Errorf("a folder carrying %s has role %q, want none", attr, got)
		}
	}
}

// The servers without SPECIAL-USE are the old ones, and the old ones are
// exactly where a Turkish corporate account is likely to live.
func TestFolderNameIsTheFallbackWhenTheServerSaysNothing(t *testing.T) {
	cases := []struct {
		name string
		want FolderRole
	}{
		{"Sent", RoleSent},
		{"Sent Items", RoleSent},
		{"Gönderilmiş Öğeler", RoleSent},
		{"Gonderilenler", RoleSent},
		{"Drafts", RoleDrafts},
		{"Taslaklar", RoleDrafts},
		{"Trash", RoleTrash},
		{"Deleted Items", RoleTrash},
		{"Çöp Kutusu", RoleTrash},
		{"Silinmiş Öğeler", RoleTrash},
		{"Archive", RoleArchive},
		{"Arşiv", RoleArchive},
		{"Junk E-mail", RoleJunk},
		{"Spam", RoleJunk},
		{"İstenmeyen", RoleJunk},
		{"Muhasebe", RoleNone},
		{"2026 Projeleri", RoleNone},
	}

	for _, c := range cases {
		f := Folder{Name: c.name, Path: c.name}
		if got := f.Role(); got != c.want {
			t.Errorf("%q has role %q, want %q", c.name, got, c.want)
		}
	}
}

// A wrong guess from the name must not override what the server said. A
// mailbox named "Sent to legal" carrying \Archive is an archive folder.
func TestTheAttributeBeatsTheName(t *testing.T) {
	f := Folder{Name: "Sent to legal", Path: "Sent to legal", Attributes: []string{"\\Archive"}}
	if got := f.Role(); got != RoleArchive {
		t.Errorf("Role() = %q, want the attribute to win with %q", got, RoleArchive)
	}
}

func TestInboxIsAlwaysTheInbox(t *testing.T) {
	// IMAP defines INBOX as the one case-insensitive mailbox name.
	for _, path := range []string{"INBOX", "inbox", "Inbox"} {
		if got := (Folder{Name: path, Path: path}).Role(); got != RoleInbox {
			t.Errorf("%q has role %q, want inbox", path, got)
		}
	}
}

// Alphabetical order scatters the six mailboxes used every day through a list
// of project folders — and on a Turkish account it does not even scatter them
// to the same places as on an English one.
func TestSortFoldersPutsTheEverydayMailboxesFirst(t *testing.T) {
	folders := []Folder{
		{Name: "Arşiv", Path: "Arşiv"},
		{Name: "Muhasebe", Path: "Muhasebe"},
		{Name: "Çöp Kutusu", Path: "Çöp Kutusu"},
		{Name: "INBOX", Path: "INBOX"},
		{Name: "Zeyilname", Path: "Zeyilname"},
		{Name: "Gönderilmiş Öğeler", Path: "Gönderilmiş Öğeler"},
		{Name: "Taslaklar", Path: "Taslaklar"},
	}

	SortFolders(folders)

	want := []FolderRole{RoleInbox, RoleDrafts, RoleSent, RoleArchive, RoleTrash, RoleNone, RoleNone}
	for i, role := range want {
		if got := folders[i].Role(); got != role {
			t.Errorf("position %d holds %q (%q), want role %q", i, folders[i].Name, got, role)
		}
	}
}

// The folders with no role keep the order they arrived in, which is the path
// order the database produced. Re-sorting them here would be a second opinion
// about alphabetisation, and Go's sort does not know about Turkish collation.
func TestSortFoldersLeavesOrdinaryFoldersWhereTheyWere(t *testing.T) {
	folders := []Folder{
		{Name: "Bordro", Path: "Bordro"},
		{Name: "Ar-Ge", Path: "Ar-Ge"},
		{Name: "INBOX", Path: "INBOX"},
		{Name: "Cari", Path: "Cari"},
	}

	SortFolders(folders)

	got := []string{folders[1].Name, folders[2].Name, folders[3].Name}
	want := []string{"Bordro", "Ar-Ge", "Cari"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ordinary folders came out as %v, want %v", got, want)
			break
		}
	}
}
