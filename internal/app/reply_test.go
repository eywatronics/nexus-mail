package app

import (
	"context"
	"net/mail"
	"strings"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// replyFixture gives a service holding one message with a full set of
// recipients, and the account's own identities.
func replyFixture(t *testing.T) (*MailService, int64) {
	return replyFixtureWith(t, nil)
}

// replyFixtureWith is the same message with a Reply-To on it, which is the
// mailing-list case: the list posts under the member's name and asks for
// answers to come back to the list.
func replyFixtureWith(t *testing.T, replyTo []model.Address) (*MailService, int64) {
	t.Helper()

	svc, _, db := newTestService(t, bodyBackend{html: "<p>özgün gövde</p>"})
	ctx := context.Background()

	acct, err := svc.AddPasswordAccount("ben@example.com", "Ben",
		"imap.example.com", 993, "tls", "smtp.example.com", 587, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}
	if _, err := db.InsertIdentity(ctx, model.Identity{
		AccountID: acct.ID, Email: "ben@example.com", DisplayName: "Ben",
	}); err != nil {
		t.Fatalf("InsertIdentity() error: %v", err)
	}

	folders, _ := svc.ListFolders(acct.ID)
	var inbox int64
	for _, f := range folders {
		if f.IsInbox {
			inbox = f.ID
		}
	}

	// A message addressed to a group, with the reader among them.
	if err := db.UpsertMessages(ctx, inbox, []model.Message{{
		AccountID: acct.ID, FolderID: inbox, UID: 500,
		MessageID:  "parent@example.com",
		References: []string{"root@example.com"},
		Subject:    "Toplantı",
		From:       model.Address{Name: "Yazan", Addr: "yazan@example.com"},
		To: []model.Address{
			{Name: "Ben", Addr: "ben@example.com"},
			{Name: "Biri", Addr: "biri@example.com"},
		},
		Cc:           []model.Address{{Addr: "baskasi@example.com"}},
		ReplyTo:      replyTo,
		Date:         time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC),
		InternalDate: time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	msgs, _ := svc.ListMessages(inbox, 10, 0, false)
	if len(msgs) == 0 {
		t.Fatal("the fixture message is not there")
	}
	return svc, msgs[0].ID
}

func TestAReplyGoesToTheSenderAndNobodyElse(t *testing.T) {
	svc, id := replyFixture(t)

	got, err := svc.ReplyDraft(id, false)
	if err != nil {
		t.Fatalf("ReplyDraft() error: %v", err)
	}

	if !strings.Contains(got.To, "yazan@example.com") {
		t.Errorf("To = %q", got.To)
	}
	if got.Cc != "" {
		t.Errorf("a plain reply put %q on the Cc line", got.Cc)
	}
}

// Without the exclusion the reader mails themselves every time they answer a
// group — and on a thread of ten replies that is ten copies in their own inbox.
func TestReplyAllKeepsEverybodyButTheReader(t *testing.T) {
	svc, id := replyFixture(t)

	got, err := svc.ReplyDraft(id, true)
	if err != nil {
		t.Fatalf("ReplyDraft() error: %v", err)
	}

	if strings.Contains(got.Cc, "ben@example.com") {
		t.Errorf("the reader is on their own Cc line: %q", got.Cc)
	}
	for _, want := range []string{"biri@example.com", "baskasi@example.com"} {
		if !strings.Contains(got.Cc, want) {
			t.Errorf("%s was dropped from the Cc line: %q", want, got.Cc)
		}
	}
	// And the sender is on To, not repeated on Cc.
	if strings.Contains(got.Cc, "yazan@example.com") {
		t.Errorf("the sender appears twice: To=%q Cc=%q", got.To, got.Cc)
	}
}

// Somebody who answers support@ from their personal mailbox would otherwise
// find support@ on the Cc line of every reply-all they send.
func TestReplyAllExcludesEveryIdentityNotJustTheAccountAddress(t *testing.T) {
	svc, id := replyFixture(t)
	ctx := context.Background()

	// A second identity, which is also one of the recipients.
	if _, err := svc.store.InsertIdentity(ctx, model.Identity{
		AccountID: 1, Email: "biri@example.com", DisplayName: "Destek",
	}); err != nil {
		t.Fatalf("InsertIdentity() error: %v", err)
	}

	got, err := svc.ReplyDraft(id, true)
	if err != nil {
		t.Fatalf("ReplyDraft() error: %v", err)
	}
	if strings.Contains(got.Cc, "biri@example.com") {
		t.Errorf("a second identity of the reader's is on the Cc line: %q", got.Cc)
	}
	if !strings.Contains(got.Cc, "baskasi@example.com") {
		t.Errorf("an unrelated recipient was dropped: %q", got.Cc)
	}
}

// Threading is what makes the reply appear under what it answers.
func TestAReplyCarriesTheThreadingHeaders(t *testing.T) {
	svc, id := replyFixture(t)

	got, err := svc.ReplyDraft(id, false)
	if err != nil {
		t.Fatalf("ReplyDraft() error: %v", err)
	}

	if got.InReplyTo != "parent@example.com" {
		t.Errorf("InReplyTo = %q", got.InReplyTo)
	}
	if len(got.References) != 2 ||
		got.References[0] != "root@example.com" ||
		got.References[1] != "parent@example.com" {
		t.Errorf("References = %v", got.References)
	}
}

func TestAReplySubjectIsPrefixedOnce(t *testing.T) {
	svc, id := replyFixture(t)

	got, _ := svc.ReplyDraft(id, false)
	if got.Subject != "Re: Toplantı" {
		t.Errorf("Subject = %q", got.Subject)
	}
}

func TestTheOriginalIsQuotedUnderTheReply(t *testing.T) {
	svc, id := replyFixture(t)

	got, err := svc.ReplyDraft(id, false)
	if err != nil {
		t.Fatalf("ReplyDraft() error: %v", err)
	}

	if !strings.Contains(got.Quoted, "Yazan") {
		t.Errorf("the attribution does not name the sender:\n%s", got.Quoted)
	}
	if !strings.Contains(got.Quoted, "> özgün gövde") {
		t.Errorf("the body was not quoted:\n%s", got.Quoted)
	}
}

// A forward starts a new conversation with somebody who was not in the old
// one, and threading it would file it in a thread they have never seen.
func TestAForwardStartsAFreshConversation(t *testing.T) {
	svc, id := replyFixture(t)

	got, err := svc.ForwardDraft(id)
	if err != nil {
		t.Fatalf("ForwardDraft() error: %v", err)
	}

	if got.To != "" || got.Cc != "" {
		t.Errorf("a forward arrived pre-addressed: To=%q Cc=%q", got.To, got.Cc)
	}
	if got.InReplyTo != "" || len(got.References) != 0 {
		t.Errorf("a forward was threaded under the original: %q %v",
			got.InReplyTo, got.References)
	}
	if got.Subject != "Fwd: Toplantı" {
		t.Errorf("Subject = %q", got.Subject)
	}
}

// The recipient is being shown a message, not a conversation they were part
// of, and the headers are the part they need to judge it.
func TestAForwardCarriesTheOriginalHeaders(t *testing.T) {
	svc, id := replyFixture(t)

	got, err := svc.ForwardDraft(id)
	if err != nil {
		t.Fatalf("ForwardDraft() error: %v", err)
	}

	for _, want := range []string{
		"Forwarded message", "yazan@example.com", "Toplantı", "ben@example.com",
	} {
		if !strings.Contains(got.Quoted, want) {
			t.Errorf("%q is missing from the forwarded block:\n%s", want, got.Quoted)
		}
	}
}

// A display name with a comma in it parses as two addresses unless it is
// quoted, and that is exactly the shape a name takes in a corporate directory.
func TestADisplayNameWithACommaIsQuoted(t *testing.T) {
	got := model.Address{Name: "Kabatepe, Ismet", Addr: "u@example.com"}.String()

	if !strings.HasPrefix(got, `"Kabatepe, Ismet"`) {
		t.Errorf("Address.String() = %s; the name was not quoted", got)
	}

	// And it survives being read back, which is what the composer depends on:
	// it sends the line as typed and the backend parses it.
	parsed, err := mail.ParseAddressList(got)
	if err != nil {
		t.Fatalf("the formatted address does not parse: %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("it parsed as %d addresses: %v", len(parsed), parsed)
	}
	if parsed[0].Name != "Kabatepe, Ismet" {
		t.Errorf("the name came back as %q", parsed[0].Name)
	}
}

// A non-ASCII name takes the other route: RFC 2047 encodes it, and the comma
// is safely inside the encoded word rather than needing quotes around it.
func TestANonASCIIDisplayNameIsEncoded(t *testing.T) {
	got := model.Address{Name: "Kabatepe, İsmet", Addr: "u@example.com"}.String()

	if !strings.Contains(got, "=?utf-8?") {
		t.Errorf("Address.String() = %s; the name was not encoded", got)
	}

	parsed, err := mail.ParseAddressList(got)
	if err != nil {
		t.Fatalf("the formatted address does not parse: %v", err)
	}
	if len(parsed) != 1 || parsed[0].Name != "Kabatepe, İsmet" {
		t.Errorf("the name came back as %v", parsed)
	}
}

func TestAnAddressWithNoNameIsJustTheAddress(t *testing.T) {
	if got := (model.Address{Addr: "u@example.com"}).String(); got != "u@example.com" {
		t.Errorf("Address.String() = %q", got)
	}
}

func TestReplyingToSomethingThatIsNotThereIsReported(t *testing.T) {
	svc, _ := replyFixture(t)

	if _, err := svc.ReplyDraft(9999, false); err == nil {
		t.Error("ReplyDraft() succeeded for a message that does not exist")
	}
	if _, err := svc.ForwardDraft(9999); err == nil {
		t.Error("ForwardDraft() succeeded for a message that does not exist")
	}
}

// The header exists to be obeyed. A list sets Reply-To so that answers reach
// the list, and answering the person who happened to post takes the
// conversation off it without anybody noticing.
func TestAReplyGoesWhereTheAuthorAskedRatherThanWhereItCameFrom(t *testing.T) {
	svc, id := replyFixtureWith(t, []model.Address{
		{Name: "Liste", Addr: "liste@example.com"},
	})

	got, err := svc.ReplyDraft(id, false)
	if err != nil {
		t.Fatalf("ReplyDraft() error: %v", err)
	}

	if !strings.Contains(got.To, "liste@example.com") {
		t.Errorf("To = %q, want the list", got.To)
	}
	if strings.Contains(got.To, "yazan@example.com") {
		t.Errorf("To = %q; the sender was asked not to be written to", got.To)
	}
}

// And the sender does not reappear on the Cc line of a reply-all. Sending to
// both puts a copy on the list and a private copy on the person who posted,
// which is the outcome Reply-To was set to avoid.
func TestReplyAllDoesNotPutTheSenderBackWhenReplyToRedirected(t *testing.T) {
	svc, id := replyFixtureWith(t, []model.Address{
		{Name: "Liste", Addr: "liste@example.com"},
	})

	got, err := svc.ReplyDraft(id, true)
	if err != nil {
		t.Fatalf("ReplyDraft() error: %v", err)
	}

	if strings.Contains(got.To+got.Cc, "yazan@example.com") {
		t.Errorf("the sender is on To=%q Cc=%q", got.To, got.Cc)
	}
	// The other people on the thread are still there; redirecting the answer
	// is not the same as dropping everybody else.
	if !strings.Contains(got.Cc, "biri@example.com") {
		t.Errorf("Cc = %q, want the other recipients", got.Cc)
	}
	// And the reader is still not writing to themselves.
	if strings.Contains(got.Cc, "ben@example.com") {
		t.Errorf("Cc = %q includes the reader", got.Cc)
	}
}

// A forward is judged on its headers, and Reply-To is stored only when it
// differs from the sender — so the line appears exactly when it is news.
func TestAForwardShowsAReplyToThatDiffersFromTheSender(t *testing.T) {
	svc, id := replyFixtureWith(t, []model.Address{
		{Name: "Liste", Addr: "liste@example.com"},
	})

	got, err := svc.ForwardDraft(id)
	if err != nil {
		t.Fatalf("ForwardDraft() error: %v", err)
	}
	if !strings.Contains(got.Quoted, "Reply-To: ") {
		t.Errorf("the forwarded header block has no Reply-To line:\n%s", got.Quoted)
	}
}

func TestAForwardOfAnOrdinaryMessageHasNoReplyToLine(t *testing.T) {
	svc, id := replyFixture(t)

	got, err := svc.ForwardDraft(id)
	if err != nil {
		t.Fatalf("ForwardDraft() error: %v", err)
	}
	if strings.Contains(got.Quoted, "Reply-To:") {
		t.Errorf("a Reply-To line appeared with nothing to report:\n%s", got.Quoted)
	}
}
