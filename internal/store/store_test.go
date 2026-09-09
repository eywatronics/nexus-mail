package store

import (
	"testing"
)

func TestOpenAppliesMigrationsAndPragmas(t *testing.T) {
	s := openTestStore(t)

	var mode string
	if err := s.Read().QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("journal_mode query: %v", err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode = %q, want wal", mode)
	}

	var fk int
	if err := s.Read().QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatalf("foreign_keys query: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	tables := []string{
		"accounts", "folders", "messages", "message_bodies",
		"attachments", "operations", "fts_messages",
	}
	for _, table := range tables {
		var n int
		err := s.Read().QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE name = ?", table).Scan(&n)
		if err != nil {
			t.Fatalf("sqlite_master query for %s: %v", table, err)
		}
		if n != 1 {
			t.Errorf("table %s: found %d entries, want 1", table, n)
		}
	}
}

// The FTS index is maintained entirely by triggers. If they are missing, search
// stays empty and nothing else complains.
func TestOpenCreatesFTSTriggers(t *testing.T) {
	s := openTestStore(t)

	triggers := []string{
		"messages_fts_insert", "messages_fts_delete", "messages_fts_update",
	}
	for _, trigger := range triggers {
		var n int
		err := s.Read().QueryRow(
			"SELECT count(*) FROM sqlite_master WHERE type = 'trigger' AND name = ?",
			trigger).Scan(&n)
		if err != nil {
			t.Fatalf("sqlite_master query for %s: %v", trigger, err)
		}
		if n != 1 {
			t.Errorf("trigger %s: found %d, want 1", trigger, n)
		}
	}
}

// M3's queue worker compares this stamp against the folder's current
// UIDVALIDITY before applying an operation. Without it, a mailbox the server
// recreated while we were offline would have us act on UIDs that now refer to
// entirely different messages.
func TestOperationsTableCarriesUIDValidityStamp(t *testing.T) {
	s := openTestStore(t)

	for _, column := range []string{"folder_id", "uid_validity"} {
		var n int
		err := s.Read().QueryRow(
			`SELECT count(*) FROM pragma_table_info('operations') WHERE name = ?`,
			column).Scan(&n)
		if err != nil {
			t.Fatalf("pragma_table_info query: %v", err)
		}
		if n != 1 {
			t.Errorf("operations.%s is missing; M3 cannot guard against a UIDVALIDITY change without it", column)
		}
	}
}

func TestOpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()

	s1, err := Open(dir)
	if err != nil {
		t.Fatalf("first Open() error: %v", err)
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	s2, err := Open(dir)
	if err != nil {
		t.Fatalf("second Open() error: %v", err)
	}
	t.Cleanup(func() {
		if err := s2.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	})

	var version int
	if err := s2.Read().QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("user_version query: %v", err)
	}
	if version != 1 {
		t.Errorf("user_version = %d, want 1", version)
	}
}

func TestWritePoolHasSingleConnection(t *testing.T) {
	s := openTestStore(t)

	if got := s.Write().Stats().MaxOpenConnections; got != 1 {
		t.Errorf("write pool MaxOpenConnections = %d, want 1", got)
	}
}

func TestForeignKeysCascadeOnAccountDelete(t *testing.T) {
	s := openTestStore(t)

	if _, err := s.Write().Exec(
		`INSERT INTO accounts (email, provider, auth_kind, imap_host, imap_port, secret_ref, created_at)
		 VALUES ('a@example.com', 'generic', 'password', 'h', 993, 'ref', 0)`); err != nil {
		t.Fatalf("insert account: %v", err)
	}
	if _, err := s.Write().Exec(
		`INSERT INTO folders (account_id, name, path) VALUES (1, 'INBOX', 'INBOX')`); err != nil {
		t.Fatalf("insert folder: %v", err)
	}
	if _, err := s.Write().Exec(
		`INSERT INTO messages (account_id, folder_id, uid, subject) VALUES (1, 1, 1, 'x')`); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	// Removing an account must not leave orphaned folders and messages behind.
	// This only works if foreign_keys is actually on, which the pragma test
	// above asserts separately.
	if _, err := s.Write().Exec(`DELETE FROM accounts WHERE id = 1`); err != nil {
		t.Fatalf("delete account: %v", err)
	}

	for _, table := range []string{"folders", "messages"} {
		var n int
		if err := s.Read().QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != 0 {
			t.Errorf("%s still holds %d rows after the account was deleted", table, n)
		}
	}
}
