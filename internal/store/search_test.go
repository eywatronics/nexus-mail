package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// Free text typed into a search box is not an FTS5 expression. FTS5 has its
// own grammar — column filters, NEAR, AND/OR/NOT, quoting, prefix stars — and
// handing user input to MATCH raw means a colon or a stray quote returns a
// syntax error instead of results, while a word like AND silently changes what
// the query means. Every case below is something a person types by accident.
func TestFTSQueryEscapesWhatPeopleActuallyType(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"single term gets a prefix star", "fatura", `"fatura"*`},
		{"two terms are ANDed, only the last is a prefix", "şubat fatura", `"şubat" AND "fatura"*`},
		{"a colon is not a column filter", "from:ahmet", `"from" AND "ahmet"*`},
		{"a bare quote does not open a phrase", `he says "ok`, `"he" AND "says" AND "ok"*`},
		{"a star is not a prefix operator the user chose", "fat*ura", `"fat" AND "ura"*`},
		{"FTS5 keywords are terms, not operators", "belge AND makbuz", `"belge" AND "AND" AND "makbuz"*`},
		{"punctuation alone is not a search", "  :*\" ", ""},
		{"empty input searches for nothing", "", ""},
		{"surrounding whitespace is not a term", "  fatura  ", `"fatura"*`},
		{"digits are searchable", "2024 fatura", `"2024" AND "fatura"*`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ftsQuery(tt.input); got != tt.want {
				t.Errorf("ftsQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Turkish's dotless i is the one letter FTS5 cannot fold on its own, so the
// query builder expands it. Everything else — Ş, Ğ, Ç, Ö, Ü, İ — the tokenizer
// already handles, and expanding those too would multiply the expression for
// nothing.
func TestFTSQueryExpandsTheDottedAndDotlessI(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"one i becomes a choice of both letters",
			"hı",
			`("hi"* OR "hı"*)`,
		},
		{
			"every position is independent",
			"isi",
			`("isi"* OR "ısi"* OR "isı"* OR "ısı"*)`,
		},
		{
			"only the final term carries the prefix star",
			"isi fatura",
			`("isi" OR "ısi" OR "isı" OR "ısı") AND "fatura"*`,
		},
		{
			"a term past the variant cap is searched as typed",
			"iyileştirilmiş",
			`"iyileştirilmiş"*`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ftsQuery(tt.input); got != tt.want {
				t.Errorf("ftsQuery(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// The escaping above is only worth anything if SQLite accepts the result. A
// unit test on the string can agree with itself and still produce expressions
// FTS5 rejects, so every one of these goes through a real MATCH.
func TestSearchAcceptsEveryQueryTheEscaperProduces(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, fid, []model.Message{
		{AccountID: acct, FolderID: fid, UID: 1, Subject: "Şubat faturası",
			From: model.Address{Addr: "muhasebe@example.com"}, InternalDate: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	for _, input := range []string{
		"fatura", "from:ahmet", `he said "hi`, "fat*ura", "invoice AND receipt",
		"  :*\" ", "", "NEAR(a b)", "a OR b", "-negated", "^caret", "(unbalanced",
	} {
		if _, err := s.SearchMessages(ctx, acct, input, 50); err != nil {
			t.Errorf("SearchMessages(%q) error: %v", input, err)
		}
	}
}

func TestSearchFindsMessagesBySubjectSenderAndSnippet(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, fid, []model.Message{
		{AccountID: acct, FolderID: fid, UID: 1, Subject: "Şubat faturası",
			From:    model.Address{Name: "Muhasebe", Addr: "muhasebe@example.com"},
			Snippet: "ekte bulabilirsiniz", InternalDate: time.Unix(100, 0)},
		{AccountID: acct, FolderID: fid, UID: 2, Subject: "Toplantı notları",
			From:    model.Address{Name: "Zeynep", Addr: "zeynep@example.com"},
			Snippet: "salı günü", InternalDate: time.Unix(200, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	cases := map[string]uint32{
		"faturası": 1, // subject
		"muhasebe": 1, // sender address
		"ekte":     1, // snippet
		"toplantı": 2,
		"zeynep":   2,
		"salı":     2,
	}
	for query, wantUID := range cases {
		got, err := s.SearchMessages(ctx, acct, query, 50)
		if err != nil {
			t.Fatalf("SearchMessages(%q) error: %v", query, err)
		}
		if len(got) != 1 {
			t.Errorf("SearchMessages(%q) returned %d messages, want 1", query, len(got))
			continue
		}
		if got[0].UID != wantUID {
			t.Errorf("SearchMessages(%q) found UID %d, want %d", query, got[0].UID, wantUID)
		}
	}
}

// Search is how someone finds a message they half-remember, so it has to work
// while they are still typing. Requiring the whole word before anything
// appears makes the feature feel broken rather than strict.
func TestSearchMatchesOnAPrefixSoResultsAppearWhileTyping(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, fid, []model.Message{
		{AccountID: acct, FolderID: fid, UID: 1, Subject: "Mutabakat dosyası",
			InternalDate: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	for _, prefix := range []string{"m", "mu", "muta", "mutabakat"} {
		got, err := s.SearchMessages(ctx, acct, prefix, 50)
		if err != nil {
			t.Fatalf("SearchMessages(%q) error: %v", prefix, err)
		}
		if len(got) != 1 {
			t.Errorf("typing %q found %d messages, want 1", prefix, len(got))
		}
	}

	// Only the last term is a prefix. An earlier term matching by prefix would
	// make "top not" match "topluluk notları", which is not what was asked.
	got, err := s.SearchMessages(ctx, acct, "muta dosyası", 50)
	if err != nil {
		t.Fatalf("SearchMessages() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a non-final term matched by prefix; got %d messages, want 0", len(got))
	}
}

// Turkish users type on keyboards that have the diacritics and on keyboards
// that do not. FTS5's tokenizer folds diacritics on both sides, so "subat"
// finds "Şubat" — worth locking down, because losing it would look like a
// broken search to half the intended audience.
func TestSearchFindsTurkishWordsTypedWithoutDiacritics(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, fid, []model.Message{
		{AccountID: acct, FolderID: fid, UID: 1, Subject: "Şubat ayı mutabakatı",
			InternalDate: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	for _, query := range []string{
		"Şubat", "şubat", "subat", "SUBAT", // handled by the tokenizer
		"mutabakatı", "mutabakati", "MUTABAKATI", // handled by the expansion
	} {
		got, err := s.SearchMessages(ctx, acct, query, 50)
		if err != nil {
			t.Fatalf("SearchMessages(%q) error: %v", query, err)
		}
		if len(got) != 1 {
			t.Errorf("SearchMessages(%q) found %d messages, want 1", query, len(got))
		}
	}
}

// Two accounts in one database is the normal case, and a search that crosses
// between them shows a person their work mail while they are looking through
// their personal one.
func TestSearchDoesNotCrossAccountBoundaries(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	work, workFolder := seedInbox(t, s)
	personal, err := s.InsertAccount(ctx, model.Account{
		Email: "personal@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "imap.example.com", IMAPPort: 993,
		SecretRef: "personal", CreatedAt: time.Unix(1, 0),
	})
	if err != nil {
		t.Fatalf("InsertAccount() error: %v", err)
	}

	if err := s.UpsertFolders(ctx, personal, []model.Folder{{Path: "INBOX", Name: "INBOX"}}); err != nil {
		t.Fatalf("UpsertFolders() error: %v", err)
	}
	personalFolder := folderIDByPath(t, s, personal, "INBOX")

	if err := s.UpsertMessages(ctx, workFolder, []model.Message{
		{AccountID: work, FolderID: workFolder, UID: 1, Subject: "Bordro dosyası",
			InternalDate: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages(work) error: %v", err)
	}
	if err := s.UpsertMessages(ctx, personalFolder, []model.Message{
		{AccountID: personal, FolderID: personalFolder, UID: 1, Subject: "Bordro sorusu",
			InternalDate: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages(personal) error: %v", err)
	}

	got, err := s.SearchMessages(ctx, work, "bordro", 50)
	if err != nil {
		t.Fatalf("SearchMessages() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("search returned %d messages, want 1 from the work account", len(got))
	}
	if got[0].AccountID != work {
		t.Errorf("search returned a message from account %d, want %d", got[0].AccountID, work)
	}
}

// Results come back newest first and capped, because a search across a mailbox
// with a hundred thousand messages otherwise tries to render all of them.
func TestSearchReturnsNewestFirstAndRespectsTheLimit(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	var msgs []model.Message
	for i := 1; i <= 10; i++ {
		msgs = append(msgs, model.Message{
			AccountID: acct, FolderID: fid, UID: uint32(i),
			Subject:      "Rapor",
			InternalDate: time.Unix(int64(i)*100, 0),
		})
	}
	if err := s.UpsertMessages(ctx, fid, msgs); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	got, err := s.SearchMessages(ctx, acct, "rapor", 3)
	if err != nil {
		t.Fatalf("SearchMessages() error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("search returned %d messages, want 3", len(got))
	}
	for i, want := range []uint32{10, 9, 8} {
		if got[i].UID != want {
			t.Errorf("result %d is UID %d, want %d", i, got[i].UID, want)
		}
	}
}

// A deleted message must leave the results as well as the index. This is the
// end-to-end version of the trigger test: it goes through the query path
// people actually use rather than counting rows in fts_messages.
func TestSearchStopsFindingMessagesAfterTheirFolderIsReset(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, fid := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, fid, []model.Message{
		{AccountID: acct, FolderID: fid, UID: 1, Subject: "Silinecek kayıt",
			InternalDate: time.Unix(1, 0)},
	}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	if got, _ := s.SearchMessages(ctx, acct, "silinecek", 50); len(got) != 1 {
		t.Fatalf("precondition failed: found %d messages, want 1", len(got))
	}

	if err := s.ResetFolder(ctx, fid, 99); err != nil {
		t.Fatalf("ResetFolder() error: %v", err)
	}

	got, err := s.SearchMessages(ctx, acct, "silinecek", 50)
	if err != nil {
		t.Fatalf("SearchMessages() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("reset folder's mail is still searchable: %d results", len(got))
	}
	assertFTSIntegrity(t, s)
}
