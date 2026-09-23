package imapx

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"

	"nexusmail/internal/model"
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

// FetchFlags is the cheap half of a delta sync: it asks what changed without
// pulling envelopes. A message going from unread to read produces no new
// header data, and refetching the envelope to learn that would multiply the
// traffic the delta path exists to avoid.
func TestFetchFlagsReturnsUIDsAndFlagsWithoutEnvelopes(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("First", "a@example.com", "<p>1</p>"))
	appendMessage(t, user, "INBOX", htmlMessage("Second", "b@example.com", "<p>2</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	updates, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if err != nil {
		t.Fatalf("FetchFlags() error: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("FetchFlags() returned %d updates, want 2", len(updates))
	}
	for _, u := range updates {
		if u.UID == 0 {
			t.Error("UID = 0; the store keys the update on it")
		}
	}
}

// The UID list this returns is the other half of detecting a deletion: what
// the server still has, against what we hold. A range that under-reports would
// make the caller delete mail that is still there.
func TestFetchFlagsHonoursTheUIDRange(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	for i := 0; i < 4; i++ {
		appendMessage(t, user, "INBOX", htmlMessage("Msg", "a@example.com", "<p>x</p>"))
	}

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	all, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if err != nil {
		t.Fatalf("FetchFlags(all) error: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("the whole mailbox has %d messages, want 4", len(all))
	}

	narrowed, err := be.FetchFlags(ctx, UIDRange{Start: all[1].UID, End: all[2].UID}, 0)
	if err != nil {
		t.Fatalf("FetchFlags(range) error: %v", err)
	}
	if len(narrowed) != 2 {
		t.Errorf("the narrowed range returned %d updates, want 2", len(narrowed))
	}
}

// An empty mailbox is not an error. Delta sync runs against every folder on
// every pass, including the ones nobody has ever put a message in.
func TestFetchFlagsOnAnEmptyMailbox(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	updates, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if err != nil {
		t.Fatalf("FetchFlags() error: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("FetchFlags() returned %d updates for an empty mailbox", len(updates))
	}
}

// Flags the server actually set must survive the round trip; the whole point
// of the call is learning that something is now read.
func TestFetchFlagsReportsSystemFlags(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("Read me", "a@example.com", "<p>x</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	// The flag is set deliberately, the way marking a message read does it.
	// This test used to reach it by fetching the body and relying on the
	// server's \Seen side effect, which is exactly the side effect the client
	// now avoids.
	if err := be.StoreFlags(ctx, []uint32{1}, []string{model.FlagSeen}, true); err != nil {
		t.Fatalf("StoreFlags() error: %v", err)
	}

	after, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if err != nil {
		t.Fatalf("FetchFlags() error: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("got %d updates, want 1", len(after))
	}
	if !(model.Message{Flags: after[0].Flags}).HasFlag(model.FlagSeen) {
		t.Errorf("flags = %v, want the seen flag reported", after[0].Flags)
	}
}

// IDLE is what makes new mail appear without polling. The assertion that
// matters is that the call actually returns when the server has something to
// say — a version that always timed out would look identical from the outside
// until someone waited half an hour for a message.
func TestIdleWakesWhenAMessageArrives(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps(imap.CapIdle))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	delivered := make(chan struct{})
	go func() {
		defer close(delivered)
		// Long enough that IDLE is certainly established first; short enough
		// that the test does not become a sleep.
		time.Sleep(150 * time.Millisecond)
		appendMessage(t, user, "INBOX", htmlMessage("Yeni", "a@example.com", "<p>x</p>"))
	}()

	changed, err := be.Idle(ctx)
	if err != nil {
		t.Fatalf("Idle() error: %v", err)
	}
	if !changed {
		t.Error("Idle() reported no change after a message was delivered")
	}
	<-delivered
}

// The supervisor cancels the context to shut the loop down. Blocking past that
// would leave a goroutine holding a connection open after the window closed.
func TestIdleReturnsWhenTheContextIsCancelled(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps(imap.CapIdle))

	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(context.Background(), "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	changed, err := be.Idle(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Idle() error = %v, want context.Canceled", err)
	}
	if changed {
		t.Error("Idle() reported a change on cancellation")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Idle() took %s to notice cancellation", elapsed)
	}
}

// A server that does not advertise IDLE has to be told so, not discovered by
// a command that hangs until the connection times out.
//
// Checked against the struct rather than a live server: what the fake
// advertises is go-imap's business (IMAP4rev2 subsumes IDLE, so a rev2 server
// cannot be made to lack it), while the refusal is ours.
func TestIdleIsRefusedWhenTheServerDoesNotAdvertiseIt(t *testing.T) {
	cl := &client{caps: Capabilities{Idle: false}}

	if _, err := cl.Idle(context.Background()); err == nil {
		t.Error("Idle() succeeded against a server without the capability")
	}
}

// Marking a message read is the most common thing a mail client does, and the
// only proof it worked is asking the server again.
func TestStoreFlagsAddsAndRemoves(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("Konu", "a@example.com", "<p>x</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	before, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if err != nil || len(before) != 1 {
		t.Fatalf("FetchFlags() = %v, %v", before, err)
	}
	uid := before[0].UID

	if err := be.StoreFlags(ctx, []uint32{uid}, []string{model.FlagFlagged}, true); err != nil {
		t.Fatalf("StoreFlags(add) error: %v", err)
	}
	if !hasFlag(t, be, uid, model.FlagFlagged) {
		t.Error("the flag was not added")
	}

	if err := be.StoreFlags(ctx, []uint32{uid}, []string{model.FlagFlagged}, false); err != nil {
		t.Fatalf("StoreFlags(remove) error: %v", err)
	}
	if hasFlag(t, be, uid, model.FlagFlagged) {
		t.Error("the flag was not removed")
	}
}

// Adding a flag twice has to be harmless: the queue retries, and a retry that
// corrupted state would make every dropped connection a data problem.
func TestStoreFlagsIsIdempotent(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("Konu", "a@example.com", "<p>x</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	got, _ := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	uid := got[0].UID

	for range 3 {
		if err := be.StoreFlags(ctx, []uint32{uid}, []string{model.FlagFlagged}, true); err != nil {
			t.Fatalf("StoreFlags() error: %v", err)
		}
	}

	after, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if err != nil {
		t.Fatalf("FetchFlags() error: %v", err)
	}
	var flagged int
	for _, f := range after[0].Flags {
		if strings.EqualFold(f, model.FlagFlagged) {
			flagged++
		}
	}
	if flagged != 1 {
		t.Errorf("the flag appears %d times after three adds", flagged)
	}
}

func TestMoveRelocatesMessages(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps(imap.CapMove))
	if err := user.Create("Arşiv", nil); err != nil {
		t.Fatalf("create Arşiv: %v", err)
	}
	appendMessage(t, user, "INBOX", htmlMessage("Taşınacak", "a@example.com", "<p>x</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	got, _ := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)

	if err := be.Move(ctx, []uint32{got[0].UID}, "Arşiv"); err != nil {
		t.Fatalf("Move() error: %v", err)
	}

	if left, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0); err != nil || len(left) != 0 {
		t.Errorf("INBOX still holds %v after the move (err %v)", left, err)
	}
	if _, err := be.Select(ctx, "Arşiv"); err != nil {
		t.Fatalf("Select(Arşiv) error: %v", err)
	}
	arrived, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if err != nil || len(arrived) != 1 {
		t.Errorf("Arşiv holds %v after the move (err %v)", arrived, err)
	}
}

func TestExpungeRemovesTheNamedMessages(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps(imap.CapUIDPlus))
	for range 2 {
		appendMessage(t, user, "INBOX", htmlMessage("Silinecek", "a@example.com", "<p>x</p>"))
	}

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}
	got, _ := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if len(got) != 2 {
		t.Fatalf("precondition failed: INBOX holds %d messages", len(got))
	}

	if err := be.Expunge(ctx, []uint32{got[0].UID}); err != nil {
		t.Fatalf("Expunge() error: %v", err)
	}

	left, err := be.FetchFlags(ctx, UIDRange{Start: 1}, 0)
	if err != nil {
		t.Fatalf("FetchFlags() error: %v", err)
	}
	if len(left) != 1 || left[0].UID != got[1].UID {
		t.Errorf("INBOX holds %v, want only the message that was not named", left)
	}
}

// An operation naming nothing would send a command with an empty set, which
// some servers reject and others answer surprisingly.
func TestWriteCommandsRejectAnEmptyUIDSet(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	if err := be.StoreFlags(ctx, nil, []string{model.FlagSeen}, true); err == nil {
		t.Error("StoreFlags() accepted an empty UID set")
	}
	if err := be.Move(ctx, nil, "INBOX"); err == nil {
		t.Error("Move() accepted an empty UID set")
	}
	if err := be.Expunge(ctx, nil); err == nil {
		t.Error("Expunge() accepted an empty UID set")
	}
}

func hasFlag(t *testing.T, be MailBackend, uid uint32, flag string) bool {
	t.Helper()

	updates, err := be.FetchFlags(context.Background(), UIDRange{Start: uid, End: uid}, 0)
	if err != nil {
		t.Fatalf("FetchFlags() error: %v", err)
	}
	if len(updates) == 0 {
		return false
	}
	for _, f := range updates[0].Flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

// The paperclip in the list only says a message has attachments. Opening one
// needs their names, types and sizes, and a part number to fetch by — none of
// which the envelope carries.
func TestAttachmentPartsAreListedFromTheBodyStructure(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", mixedMessage())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	msgs, err := be.FetchHeaders(ctx, UIDRange{Start: 1})
	if err != nil || len(msgs) != 1 {
		t.Fatalf("FetchHeaders() = %v, %v", msgs, err)
	}

	parts := msgs[0].Attachments
	if len(parts) != 1 {
		t.Fatalf("found %d attachments, want 1: %+v", len(parts), parts)
	}
	if parts[0].Filename != "rapor.pdf" {
		t.Errorf("Filename = %q, want rapor.pdf", parts[0].Filename)
	}
	if parts[0].MIMEType != "application/pdf" {
		t.Errorf("MIMEType = %q, want application/pdf", parts[0].MIMEType)
	}
	if parts[0].PartID == "" {
		t.Error("PartID is empty; there is no way to fetch the part")
	}
	if parts[0].Size == 0 {
		t.Error("Size = 0; the list cannot show how big the file is")
	}
}

// The body itself is not an attachment. Listing it would put "part 1" in every
// message's attachment list, and the reader would wonder what it is.
func TestTheBodyIsNotListedAsAnAttachment(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("Sade", "a@example.com", "<p>x</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	msgs, _ := be.FetchHeaders(ctx, UIDRange{Start: 1})
	if len(msgs[0].Attachments) != 0 {
		t.Errorf("a plain message reports %+v as attachments", msgs[0].Attachments)
	}
}

// Fetching the bytes is the whole point; the metadata only exists to get here.
func TestFetchPartReturnsTheAttachmentBytes(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", mixedMessage())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	msgs, _ := be.FetchHeaders(ctx, UIDRange{Start: 1})
	part := msgs[0].Attachments[0]

	data, err := be.FetchPart(ctx, msgs[0].UID, part.PartID, part.Encoding)
	if err != nil {
		t.Fatalf("FetchPart() error: %v", err)
	}
	if !strings.Contains(string(data), "PDF-1.4") {
		t.Errorf("the part came back as %q, want the decoded file", string(data))
	}
}

func TestFetchPartReportsAMissingPart(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", mixedMessage())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	if _, err := be.FetchPart(ctx, 4242, "1", ""); err == nil {
		t.Error("FetchPart() succeeded for a message that is not there")
	}
}

// mixedMessage is a multipart/mixed with a text body and a base64 PDF, which
// is what an ordinary mail with a file on it looks like on the wire.
func mixedMessage() string {
	return "From: Muhasebe <muhasebe@example.com>\r\n" +
		"To: " + testUser + "\r\n" +
		"Subject: Rapor ektedir\r\n" +
		"Message-ID: <rapor@example.com>\r\n" +
		"Date: Mon, 02 Feb 2026 09:30:00 +0300\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"sinir\"\r\n" +
		"\r\n" +
		"--sinir\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"Ektedir.\r\n" +
		"--sinir\r\n" +
		"Content-Type: application/pdf; name=\"rapor.pdf\"\r\n" +
		"Content-Disposition: attachment; filename=\"rapor.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		// "%PDF-1.4 test" base64-encoded.
		"JVBERi0xLjQgdGVzdA==\r\n" +
		"--sinir--\r\n"
}

// serverFlagsOf reports the flags the server currently holds for a UID.
func serverFlagsOf(t *testing.T, be MailBackend, uid uint32) []string {
	t.Helper()

	updates, err := be.FetchFlags(context.Background(), UIDRange{Start: uid, End: uid}, 0)
	if err != nil {
		t.Fatalf("FetchFlags() error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("FetchFlags() returned %d results for UID %d, want 1", len(updates), uid)
	}
	return updates[0].Flags
}

// Reading a message must not change its state on the server behind the app's
// back. A plain BODY[] fetch sets \Seen as a protocol side effect, which would
// mark a message read on every other device the moment the reading pane
// rendered it — and without going through the outgoing queue, so the app would
// not even know it had happened. Marking read is a decision the user makes;
// BODY.PEEK is what keeps it one.
func TestFetchingABodyDoesNotMarkItSeenOnTheServer(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("Peek", "a@example.com", "<p>Hi</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	if _, err := be.FetchBody(ctx, 1); err != nil {
		t.Fatalf("FetchBody() error: %v", err)
	}

	if flags := serverFlagsOf(t, be, 1); (model.Message{Flags: flags}).HasFlag(model.FlagSeen) {
		t.Errorf("flags after reading = %v, want the message still unread", flags)
	}
}

// Same rule for attachments: opening a file out of a message is not the same
// decision as having read the message.
func TestFetchingAPartDoesNotMarkItSeenOnTheServer(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", mixedMessage())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	if _, err := be.FetchPart(ctx, 1, "2", "base64"); err != nil {
		t.Fatalf("FetchPart() error: %v", err)
	}

	if flags := serverFlagsOf(t, be, 1); (model.Message{Flags: flags}).HasFlag(model.FlagSeen) {
		t.Errorf("flags after downloading an attachment = %v, want the message still unread", flags)
	}
}

// View source and "save as .eml" both need the bytes exactly as they arrived,
// headers included — not the parsed-and-reassembled version, which would no
// longer be evidence of what was actually received.
func TestFetchRawReturnsTheMessageAsItArrived(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps())
	appendMessage(t, user, "INBOX", htmlMessage("Ham", "a@example.com", "<p>Gövde</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	raw, err := be.FetchRaw(ctx, 1)
	if err != nil {
		t.Fatalf("FetchRaw() error: %v", err)
	}

	for _, want := range []string{"Subject: Ham", "Message-ID:", "Content-Type: text/html", "<p>Gövde</p>"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("raw message does not contain %q; got:\n%s", want, raw)
		}
	}
	if flags := serverFlagsOf(t, be, 1); (model.Message{Flags: flags}).HasFlag(model.FlagSeen) {
		t.Errorf("flags after viewing the source = %v, want the message still unread", flags)
	}
}

func TestFetchRawReportsAMissingMessage(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)
	if _, err := be.Select(ctx, "INBOX"); err != nil {
		t.Fatalf("Select() error: %v", err)
	}

	if _, err := be.FetchRaw(ctx, 9999); err == nil {
		t.Error("FetchRaw() succeeded for a UID that does not exist")
	}
}

// The mailbox attributes have to survive the LIST round trip, because they are
// what decides which mailbox is Sent — and on a Turkish account the name will
// not tell anyone.
//
// SPECIAL-USE itself cannot be exercised here: imapmemserver drops
// CreateOptions.SpecialUse on the floor, so no mailbox it serves can carry
// \Sent. What this proves is the wiring either side of that — the attributes
// the server does send arrive on the folder, and the client asks for the
// special-use ones when the server says it has them.
func TestFolderAttributesSurviveTheListRoundTrip(t *testing.T) {
	addr, user := startFakeServer(t, serverCaps(imap.CapSpecialUse))
	if err := user.Create("Arşiv", nil); err != nil {
		t.Fatalf("create Arşiv: %v", err)
	}
	// Subscribing is the only way to make this server attach an attribute to a
	// mailbox at all, so \Subscribed stands in for the special-use ones.
	if err := user.Subscribe("Arşiv"); err != nil {
		t.Fatalf("subscribe Arşiv: %v", err)
	}

	be := dialTestBackend(t, addr, testPass)
	if !be.Capabilities().SpecialUse {
		t.Fatal("SpecialUse was advertised but not read")
	}

	folders, err := be.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}

	var attributed int
	for _, f := range folders {
		attributed += len(f.Attributes)
	}
	if attributed == 0 {
		t.Error("no folder came back with any attribute; the mapping is not wired up")
	}
}

// A server that never mentions SPECIAL-USE must not be sent the return option:
// asking for an extension the server does not have is how a LIST turns into a
// protocol error on exactly the old servers this has to work against.
func TestSpecialUseIsNotClaimedWhenTheServerDoesNotHaveIt(t *testing.T) {
	addr, _ := startFakeServer(t, serverCaps())

	be := dialTestBackend(t, addr, testPass)
	if be.Capabilities().SpecialUse {
		t.Error("SpecialUse reported on a server that does not advertise it")
	}
	if _, err := be.ListFolders(context.Background()); err != nil {
		t.Errorf("ListFolders() failed against a server without SPECIAL-USE: %v", err)
	}
}

// A Turkish mailbox name is not ASCII, so it cannot travel on an IMAP4rev1
// connection as itself: it has to be encoded as modified UTF-7 on the way out
// and decoded on the way back. Getting that wrong turns "Gönderilmiş Öğeler"
// into "G&APY-nderilmi&AV8- &AMY-eler" in the folder list, or into a mailbox
// the server says does not exist.
//
// go-imap does the encoding, which is precisely why this is worth a test: it
// is a thing this client depends on and does not implement, and a library
// change would surface here rather than in a user's folder list.
func TestNonASCIIMailboxNamesSurviveTheWire(t *testing.T) {
	const folder = "Gönderilmiş Öğeler"

	addr, user := startFakeServer(t, serverCaps())
	if err := user.Create(folder, nil); err != nil {
		t.Fatalf("create %q: %v", folder, err)
	}
	appendMessage(t, user, folder, htmlMessage("Zeyilname", "a@example.com", "<p>x</p>"))

	ctx := context.Background()
	be := dialTestBackend(t, addr, testPass)

	folders, err := be.ListFolders(ctx)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}

	var found bool
	for _, f := range folders {
		if f.Path == folder {
			found = true
			if f.Name != folder {
				t.Errorf("Name = %q, want %q", f.Name, folder)
			}
		}
	}
	if !found {
		var paths []string
		for _, f := range folders {
			paths = append(paths, f.Path)
		}
		t.Fatalf("the folder list holds %v, want %q among them", paths, folder)
	}

	// The name has to work as a command argument too, not only in a response.
	// A client that can list a mailbox but not select it is no better off.
	if _, err := be.Select(ctx, folder); err != nil {
		t.Fatalf("Select(%q) error: %v", folder, err)
	}
	msgs, err := be.FetchHeaders(ctx, UIDRange{Start: 1})
	if err != nil || len(msgs) != 1 {
		t.Fatalf("FetchHeaders() = %v, %v", msgs, err)
	}
}

// Turkish is the case this client is most likely to meet, but the encoding is
// not language-specific and a test that only covered one alphabet would pass
// on a broken implementation of the rest.
func TestMailboxNamesInOtherAlphabetsAlsoSurvive(t *testing.T) {
	names := []string{"Входящие", "受信トレイ", "Αρχείο", "Ελληνικά"}

	addr, user := startFakeServer(t, serverCaps())
	for _, name := range names {
		if err := user.Create(name, nil); err != nil {
			t.Fatalf("create %q: %v", name, err)
		}
	}

	be := dialTestBackend(t, addr, testPass)
	folders, err := be.ListFolders(context.Background())
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}

	got := map[string]bool{}
	for _, f := range folders {
		got[f.Path] = true
	}
	for _, name := range names {
		if !got[name] {
			t.Errorf("%q did not survive the round trip", name)
		}
	}
}
