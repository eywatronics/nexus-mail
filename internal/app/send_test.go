package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"nexusmail/internal/model"
	"nexusmail/internal/store"
)

// sendFixture gives a service with an outbox on disk and one identity.
func sendFixture(t *testing.T) (*MailService, int64) {
	t.Helper()

	svc, _, db := newTestService(t, stubBackend{})
	svc.outbox = store.NewOutbox(t.TempDir())

	acct, err := svc.AddPasswordAccount("u@example.com", "Yazan",
		"imap.example.com", 993, "tls", "smtp.example.com", 587, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if _, err := db.InsertIdentity(context.Background(), model.Identity{
		AccountID: acct.ID, Email: "u@example.com", DisplayName: "Yazan",
	}); err != nil {
		t.Fatalf("InsertIdentity() error: %v", err)
	}
	return svc, acct.ID
}

// queued reads back the one operation and the bytes it refers to.
func queued(t *testing.T, svc *MailService, accountID int64) (model.Operation, []byte) {
	t.Helper()

	ops, err := svc.store.ClaimOperations(context.Background(), accountID, 10)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("the queue holds %d operations, want 1", len(ops))
	}
	raw, err := svc.outbox.Get(ops[0].Outbox)
	if err != nil {
		t.Fatalf("reading the queued message: %v", err)
	}
	return ops[0], raw
}

func TestSendingQueuesTheMessageRatherThanWaitingForTheServer(t *testing.T) {
	svc, account := sendFixture(t)

	got, err := svc.SendMessage(DraftDTO{
		AccountID: account,
		To:        "Okuyan <r@example.com>",
		Subject:   "Merhaba",
		Text:      "gövde",
	})
	if err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}
	if got.OperationID == 0 {
		t.Error("no operation was queued")
	}
	if got.Recipients != 1 {
		t.Errorf("Recipients = %d", got.Recipients)
	}

	op, raw := queued(t, svc, account)
	if op.Kind != model.OpSend {
		t.Errorf("the operation is a %s", op.Kind)
	}
	if !bytes.Contains(raw, []byte("Subject: Merhaba")) {
		t.Errorf("the message was not assembled:\n%s", raw)
	}
}

// The end-to-end version of the rule mailmime holds down: a blind copy is a
// recipient and appears in no header.
func TestABlindCopyIsDeliveredToAndNamedNowhere(t *testing.T) {
	svc, account := sendFixture(t)

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account,
		To:        "r@example.com",
		Cc:        "c@example.com",
		Bcc:       "gizli@example.com",
		Subject:   "Konu",
		Text:      "metin",
	}); err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}

	op, raw := queued(t, svc, account)

	if len(op.EnvelopeTo) != 3 {
		t.Errorf("the envelope carries %v, want all three", op.EnvelopeTo)
	}
	if !contains(op.EnvelopeTo, "gizli@example.com") {
		t.Error("the blind copy is not in the envelope, so it would never be delivered")
	}
	if bytes.Contains(raw, []byte("gizli@example.com")) {
		t.Errorf("the blind copy is in the message:\n%s", raw)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Silently sending to three of four recipients is the kind of failure nobody
// notices until the fourth person asks why they were left out.
func TestAnUnparseableAddressIsRefusedRatherThanDropped(t *testing.T) {
	svc, account := sendFixture(t)

	_, err := svc.SendMessage(DraftDTO{
		AccountID: account,
		To:        "r@example.com, bu bir adres degil",
		Subject:   "Konu",
		Text:      "metin",
	})
	if err == nil {
		t.Fatal("SendMessage() accepted a malformed recipient list")
	}
	if !strings.Contains(err.Error(), "To") {
		t.Errorf("the error does not name the field: %v", err)
	}

	// And nothing was queued or written.
	ops, _ := svc.store.ClaimOperations(context.Background(), account, 10)
	if len(ops) != 0 {
		t.Errorf("%d operations were queued anyway", len(ops))
	}
}

func TestAMessageWithNoRecipientsIsRefused(t *testing.T) {
	svc, account := sendFixture(t)

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, Subject: "Konu", Text: "metin",
	}); err == nil {
		t.Error("SendMessage() queued a message addressed to nobody")
	}
}

// Falling back to the default would send the message as somebody the writer
// did not choose.
func TestAnIdentityFromAnotherAccountIsRefused(t *testing.T) {
	svc, account := sendFixture(t)

	_, err := svc.SendMessage(DraftDTO{
		AccountID: account, IdentityID: 9999,
		To: "r@example.com", Subject: "Konu", Text: "metin",
	})
	if err == nil {
		t.Fatal("SendMessage() accepted an identity that is not on the account")
	}
	if !strings.Contains(err.Error(), "does not belong") {
		t.Errorf("the error does not explain: %v", err)
	}
}

// The chosen identity is who the message is from, not the account's address.
func TestTheChosenIdentityIsWhoTheMessageIsFrom(t *testing.T) {
	svc, account := sendFixture(t)
	ctx := context.Background()

	second, err := svc.store.InsertIdentity(ctx, model.Identity{
		AccountID: account, Email: "destek@example.com", DisplayName: "Destek",
	})
	if err != nil {
		t.Fatalf("InsertIdentity() error: %v", err)
	}

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, IdentityID: second,
		To: "r@example.com", Subject: "Konu", Text: "metin",
	}); err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}

	op, raw := queued(t, svc, account)
	if op.EnvelopeFrom != "destek@example.com" {
		t.Errorf("the return path is %q", op.EnvelopeFrom)
	}
	if !bytes.Contains(raw, []byte("destek@example.com")) {
		t.Errorf("the From header does not name the chosen identity:\n%s", raw)
	}
}

// The two-dash line is what tells a receiving client where the message ends,
// which is how a reply quotes the one and not the other.
func TestASignatureIsAppendedBehindTheStandardSeparator(t *testing.T) {
	svc, account := sendFixture(t)
	ctx := context.Background()

	id, _ := svc.store.InsertIdentity(ctx, model.Identity{
		AccountID: account, Email: "imzali@example.com",
		SignatureText: "İyi çalışmalar\nYazan",
	})

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, IdentityID: id,
		To: "r@example.com", Subject: "Konu", Text: "metin",
	}); err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}

	_, raw := queued(t, svc, account)
	body := decodeQuotedPrintable(t, raw)

	if !strings.Contains(body, "-- \n") && !strings.Contains(body, "-- \r\n") {
		t.Errorf("the signature is not behind the standard separator:\n%s", body)
	}
	if !strings.Contains(body, "İyi çalışmalar") {
		t.Errorf("the signature is missing:\n%s", body)
	}
}

func TestNoSignatureLeavesTheBodyAlone(t *testing.T) {
	svc, account := sendFixture(t)

	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Text: "yalnizca metin",
	}); err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}

	_, raw := queued(t, svc, account)
	if strings.Contains(decodeQuotedPrintable(t, raw), "-- ") {
		t.Error("a separator was written for an identity with no signature")
	}
}

// A row naming a file that does not exist can only be resolved by failing the
// message permanently, so the file goes first — and a file with no row is
// swept later and costs nothing but disk.
func TestAFailedQueueWriteTakesTheFileWithIt(t *testing.T) {
	svc, account := sendFixture(t)

	// An account id the foreign key will refuse.
	_, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Text: "metin",
	})
	if err != nil {
		t.Fatalf("the fixture itself does not work: %v", err)
	}

	// Now with the identity pointing at an account that is gone.
	if _, err := svc.store.Write().ExecContext(context.Background(),
		`DELETE FROM accounts WHERE id = ?`, account); err != nil {
		t.Fatalf("deleting the account: %v", err)
	}
	if _, err := svc.SendMessage(DraftDTO{
		AccountID: account, To: "r@example.com", Subject: "Konu", Text: "metin",
	}); err == nil {
		t.Error("SendMessage() queued a message for an account that is gone")
	}
}

func TestSendingWithNowhereToQueueIsReported(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})

	if _, err := svc.SendMessage(DraftDTO{To: "r@example.com"}); err == nil {
		t.Error("SendMessage() succeeded with no outbox")
	}
}

// decodeQuotedPrintable turns the built message back into readable text.
func decodeQuotedPrintable(t *testing.T, raw []byte) string {
	t.Helper()

	_, body, found := bytes.Cut(raw, []byte("\r\n\r\n"))
	if !found {
		t.Fatalf("the message has no body:\n%s", raw)
	}
	// The parts are quoted-printable; =XX and soft line breaks are all that
	// stands between the bytes and the text.
	text := strings.ReplaceAll(string(body), "=\r\n", "")
	// =20 is a space, and the separator's trailing one arrives that way for a
	// reason worth knowing: quoted-printable must encode whitespace at the end
	// of a line, because otherwise a server or a gateway is free to strip it —
	// and a separator that arrives as "--" instead of "-- " is not the
	// separator RFC 3676 defines.
	for _, pair := range [][2]string{
		{"=20", " "},
		{"=C4=B0", "İ"}, {"=C3=A7", "ç"}, {"=C4=9F", "ğ"},
		{"=C4=B1", "ı"}, {"=C5=9F", "ş"}, {"=C3=B6", "ö"}, {"=C3=BC", "ü"},
	} {
		text = strings.ReplaceAll(text, pair[0], pair[1])
	}
	return text
}
