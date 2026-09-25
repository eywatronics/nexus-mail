package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// The retention window caps a folder at 25,000 messages, so that is the
// worst case this client is designed to hold — and the number these
// benchmarks use.
//
// They exist to answer one question with data rather than opinion: is
// modernc.org/sqlite, the pure-Go SQLite this project uses to keep
// CGO_ENABLED=0, fast enough for search on a full mailbox? The pure-Go engine
// is reckoned to be several times slower than the C one, and if that made
// search slow the cgo ban would be costing the user something real.
const benchMailbox = 25_000

// seedMailbox fills one folder with a full retention window of messages.
func seedMailbox(b *testing.B, s *Store, folderID int64, n int) {
	b.Helper()
	ctx := context.Background()

	// Varied text, so the index is not one term repeated: a real mailbox has a
	// long tail of words and FTS5's cost depends on the term distribution.
	subjects := []string{
		"Toplantı notları", "Fatura %d ödendi", "Şubat raporu hazır",
		"Invoice %d attached", "Re: deployment window", "Yıllık izin talebi",
		"Sprint review summary", "Kargo takip numarası", "Security advisory",
		"Sözleşme yenileme hatırlatması",
	}
	senders := []string{
		"muhasebe@example.com", "ops@example.com", "ik@example.com",
		"noreply@vendor.example", "ekip@example.com",
	}

	const batch = 500
	for start := 0; start < n; start += batch {
		msgs := make([]model.Message, 0, batch)
		for i := start; i < start+batch && i < n; i++ {
			msgs = append(msgs, model.Message{
				AccountID: 1, FolderID: folderID, UID: uint32(i + 1),
				MessageID: fmt.Sprintf("m%d@example.com", i),
				Subject:   fmt.Sprintf(subjects[i%len(subjects)], i),
				From:      model.Address{Name: "Gönderen", Addr: senders[i%len(senders)]},
				Snippet: fmt.Sprintf(
					"Merhaba, %d numaralı kayıt için ekteki belgeyi inceleyip "+
						"onaylamanızı rica ederim. İyi çalışmalar dilerim.", i),
				InternalDate: time.Now().Add(-time.Duration(i) * time.Minute),
			})
		}
		if err := s.UpsertMessages(ctx, folderID, msgs); err != nil {
			b.Fatalf("seeding: %v", err)
		}
	}
}

func benchStore(b *testing.B) (*Store, int64) {
	b.Helper()

	dir := b.TempDir()
	s, err := Open(dir)
	if err != nil {
		b.Fatalf("Open() error: %v", err)
	}
	b.Cleanup(func() { _ = s.Close() })

	ctx := context.Background()
	acct, err := s.InsertAccount(ctx, model.Account{
		Email: "u@example.com", Provider: model.ProviderGeneric,
		AuthKind: model.AuthPassword, IMAPHost: "h", IMAPPort: 993,
		SecretRef: "ref", CreatedAt: time.Now(),
	})
	if err != nil {
		b.Fatalf("InsertAccount() error: %v", err)
	}
	if err := s.UpsertFolders(ctx, acct, []model.Folder{
		{Path: "INBOX", Name: "INBOX", Attributes: []string{"\\Inbox"}},
	}); err != nil {
		b.Fatalf("UpsertFolders() error: %v", err)
	}
	folders, err := s.ListFolders(ctx, acct)
	if err != nil || len(folders) == 0 {
		b.Fatalf("ListFolders() = %v, %v", folders, err)
	}
	return s, folders[0].ID
}

// BenchmarkSearchFullMailbox is the number that decides whether the pure-Go
// SQLite is costing the user anything they can feel.
//
// A search is a keystroke away from being interactive: under about 50ms it
// feels instant, and past a few hundred it is a wait.
func BenchmarkSearchFullMailbox(b *testing.B) {
	s, folder := benchStore(b)
	seedMailbox(b, s, folder, benchMailbox)
	ctx := context.Background()

	// A term that matches a tenth of the mailbox, which is the expensive
	// shape: a term matching nothing is a fast miss, and one matching
	// everything is bounded by the limit.
	for b.Loop() {
		got, err := s.SearchMessages(ctx, 1, "fatura", 50)
		if err != nil {
			b.Fatalf("SearchMessages() error: %v", err)
		}
		if len(got) == 0 {
			b.Fatal("the benchmark searched for something that is not there")
		}
	}
}

// A term nobody wrote. The index still has to be consulted, and a miss that
// is slow would be worse than a hit that is: it is what an unfinished word
// produces on every keystroke.
func BenchmarkSearchMissFullMailbox(b *testing.B) {
	s, folder := benchStore(b)
	seedMailbox(b, s, folder, benchMailbox)
	ctx := context.Background()

	for b.Loop() {
		if _, err := s.SearchMessages(ctx, 1, "zzzyokboyleboyle", 50); err != nil {
			b.Fatalf("SearchMessages() error: %v", err)
		}
	}
}

// Turkish is the case this project cares about most, and the dotless ı is the
// letter no folding rule handles for you.
func BenchmarkSearchTurkishFullMailbox(b *testing.B) {
	s, folder := benchStore(b)
	seedMailbox(b, s, folder, benchMailbox)
	ctx := context.Background()

	for b.Loop() {
		if _, err := s.SearchMessages(ctx, 1, "çalışmalar", 50); err != nil {
			b.Fatalf("SearchMessages() error: %v", err)
		}
	}
}

// Listing is what the window does on every folder change, and it is the other
// query a full mailbox could make slow.
func BenchmarkListFirstPageOfFullMailbox(b *testing.B) {
	s, folder := benchStore(b)
	seedMailbox(b, s, folder, benchMailbox)
	ctx := context.Background()

	for b.Loop() {
		got, err := s.ListMessages(ctx, folder, 50, 0)
		if err != nil {
			b.Fatalf("ListMessages() error: %v", err)
		}
		if len(got) != 50 {
			b.Fatalf("ListMessages() returned %d", len(got))
		}
	}
}
