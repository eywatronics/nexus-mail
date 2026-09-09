package imapx

import (
	"context"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

func dialTestBackend(t *testing.T, addr, password string) MailBackend {
	t.Helper()

	be, err := Dial(context.Background(), configFor(t, addr), testProvider(t, password))
	if err != nil {
		t.Fatalf("Dial() error: %v", err)
	}
	t.Cleanup(func() {
		if err := be.Close(); err != nil && !strings.Contains(err.Error(), "closed") {
			t.Errorf("Close() error: %v", err)
		}
	})
	return be
}

// Capabilities must be read after authentication, not before. This server
// advertises only IMAP4rev1, IMAP4rev2, LITERAL-, SASL-IR and AUTH=PLAIN to an
// unauthenticated client; MOVE and IDLE appear only once logged in. Reading
// them too early would have the sync engine believe the server cannot MOVE.
func TestDialReadsCapabilitiesAfterAuthentication(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())

	be := dialTestBackend(t, addr, testPass)

	caps := be.Capabilities()
	if !caps.Move {
		t.Error("Capabilities().Move = false; capabilities were read before authentication")
	}
	if !caps.Idle {
		t.Error("Capabilities().Idle = false; capabilities were read before authentication")
	}
}

// Not every server supports CONDSTORE, and a delta sync that assumes it
// silently misses changes. Detection has to report the absence honestly — that
// is what M2's fallback path branches on.
//
// This also records a limitation of the harness: imapmemserver does not
// implement CONDSTORE, and listing the capability in Options.Caps does not
// make it appear. So the CONDSTORE-present path cannot be exercised here; it
// is covered in the sync package against a fake backend we control.
func TestCapabilitiesReportAbsenceHonestly(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps(imap.CapCondStore))

	be := dialTestBackend(t, addr, testPass)

	if be.Capabilities().CondStore {
		t.Error("Capabilities().CondStore = true, but this server does not implement it")
	}
}

func TestDialRejectsWrongPassword(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())

	// A real protocol exchange, not a mocked error: the server actually
	// refuses the credentials.
	if _, err := Dial(context.Background(), configFor(t, addr), testProvider(t, "not-the-password")); err == nil {
		t.Fatal("Dial() succeeded with the wrong password")
	}
}

func TestListFoldersReturnsMailboxesWithCounts(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	if err := user.Create("Archive", nil); err != nil {
		t.Fatalf("create Archive: %v", err)
	}
	appendMessage(t, user, "INBOX", htmlMessage("One", "a@example.com", "<p>1</p>"))
	appendMessage(t, user, "INBOX", htmlMessage("Two", "b@example.com", "<p>2</p>"))

	be := dialTestBackend(t, addr, testPass)

	folders, err := be.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}

	byPath := map[string]bool{}
	var inbox *struct{ total, unread int }
	for i := range folders {
		f := folders[i]
		byPath[f.Path] = true
		if f.Name == "" {
			t.Errorf("folder %q has an empty Name", f.Path)
		}
		if f.IsInbox() {
			inbox = &struct{ total, unread int }{f.TotalCount, f.UnreadCount}
			// LIST carries STATUS return data, so counts and UIDVALIDITY
			// arrive in the same round trip rather than one STATUS per folder.
			if f.UIDValidity == 0 {
				t.Error("INBOX UIDValidity = 0; the sync engine relies on it to detect resets")
			}
		}
	}
	if !byPath["INBOX"] {
		t.Error("ListFolders() did not return INBOX")
	}
	if !byPath["Archive"] {
		t.Error("ListFolders() did not return Archive")
	}
	if inbox == nil {
		t.Fatal("no folder reported itself as the inbox")
	}
	if inbox.total != 2 {
		t.Errorf("INBOX TotalCount = %d, want 2", inbox.total)
	}
}

func TestSelectReportsUIDValidity(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("One", "a@example.com", "<p>1</p>"))

	be := dialTestBackend(t, addr, testPass)

	res, err := be.Select(context.Background(), "INBOX")
	if err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	if res.UIDValidity == 0 {
		t.Error("Select() reported UIDValidity 0; the reset check depends on it")
	}
	if res.UIDNext == 0 {
		t.Error("Select() reported UIDNext 0")
	}
	if res.NumMessages != 1 {
		t.Errorf("NumMessages = %d, want 1", res.NumMessages)
	}
}

func TestSelectRejectsAnUnknownMailbox(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())
	be := dialTestBackend(t, addr, testPass)

	if _, err := be.Select(context.Background(), "NoSuchFolder"); err == nil {
		t.Error("Select() succeeded for a mailbox that does not exist")
	}
}

func TestFetchHeadersMapsEnvelopeFields(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("Invoice", "Ali <ali@example.com>", "<p>Hello</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	msgs, err := be.FetchHeaders(ctx, UIDRange{Start: 1})
	if err != nil {
		t.Fatalf("FetchHeaders() error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("FetchHeaders() returned %d messages, want 1", len(msgs))
	}

	m := msgs[0]
	if m.Subject != "Invoice" {
		t.Errorf("Subject = %q, want Invoice", m.Subject)
	}
	if m.From.Addr != "ali@example.com" {
		t.Errorf("From.Addr = %q, want ali@example.com", m.From.Addr)
	}
	if m.From.Name != "Ali" {
		t.Errorf("From.Name = %q, want Ali", m.From.Name)
	}
	if m.UID == 0 {
		t.Error("UID = 0; the sync engine keys everything on UID")
	}
	if m.ThreadID == "" {
		t.Error("ThreadID is empty")
	}
	if m.InternalDate.IsZero() {
		t.Error("InternalDate is zero; the message list sorts on it")
	}
	if m.Size == 0 {
		t.Error("Size = 0")
	}
	// Bodies must not be pulled by FetchHeaders — splitting the two calls is
	// what keeps a thousand-message sync to kilobytes.
	if m.BodyFetched {
		t.Error("BodyFetched = true after FetchHeaders")
	}
}

func TestFetchHeadersHonoursTheUIDRange(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	for _, subject := range []string{"One", "Two", "Three", "Four"} {
		appendMessage(t, user, "INBOX", htmlMessage(subject, "a@example.com", "<p>x</p>"))
	}

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	// The initial sync asks for the last N UIDs rather than the whole mailbox,
	// so a bounded range has to work.
	msgs, err := be.FetchHeaders(ctx, UIDRange{Start: 2, End: 3})
	if err != nil {
		t.Fatalf("FetchHeaders() error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("FetchHeaders(2..3) returned %d messages, want 2", len(msgs))
	}
	for _, m := range msgs {
		if m.UID < 2 || m.UID > 3 {
			t.Errorf("UID %d fell outside the requested range 2..3", m.UID)
		}
	}
}

func TestFetchHeadersOnAnEmptyMailbox(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	// Every account has empty folders; this must not be an error.
	msgs, err := be.FetchHeaders(ctx, UIDRange{Start: 1})
	if err != nil {
		t.Fatalf("FetchHeaders() on an empty mailbox returned %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("FetchHeaders() returned %d messages from an empty mailbox", len(msgs))
	}
}

func TestFetchBodyReturnsHTML(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("Body", "b@example.com", "<p>Rendered</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	msgs, err := be.FetchHeaders(ctx, UIDRange{Start: 1})
	if err != nil {
		t.Fatalf("FetchHeaders() error: %v", err)
	}

	body, err := be.FetchBody(ctx, msgs[0].UID)
	if err != nil {
		t.Fatalf("FetchBody() error: %v", err)
	}
	if !strings.Contains(body.HTML, "Rendered") {
		t.Errorf("Body.HTML = %q, want it to contain Rendered", body.HTML)
	}
}

func TestFetchBodyReportsAMissingMessage(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	if _, err := be.FetchBody(ctx, 9999); err == nil {
		t.Error("FetchBody() succeeded for a UID that does not exist")
	}
}
