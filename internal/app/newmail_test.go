package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// newMailFixture gives a service with one synced account and its inbox.
func newMailFixture(t *testing.T) (*MailService, *recorder, model.Account, int64) {
	t.Helper()

	svc, rec, db := newTestService(t, stubBackend{})
	svc.liveNotificationPreview = true

	dto, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(dto.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	acct, err := db.GetAccount(context.Background(), dto.ID)
	if err != nil {
		t.Fatalf("GetAccount() error: %v", err)
	}
	inbox, ok := svc.inboxOf(context.Background(), acct.ID)
	if !ok {
		t.Fatal("the synced account has no inbox")
	}
	return svc, rec, acct, inbox.ID
}

// deliverLocally writes a message straight into the store, standing in for
// what a watch pass would have written.
func deliverLocally(t *testing.T, svc *MailService, acct model.Account, inbox int64,
	uid uint32, from, subject string, seen bool) {
	t.Helper()

	var flags []string
	if seen {
		flags = []string{model.FlagSeen}
	}
	err := svc.store.UpsertMessages(context.Background(), inbox, []model.Message{{
		AccountID: acct.ID, FolderID: inbox, UID: uid,
		From:    model.Address{Name: from, Addr: "x@example.com"},
		Subject: subject, InternalDate: time.Unix(int64(uid)*60, 0), Flags: flags,
	}})
	if err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
}

func newMailEvents(rec *recorder) []NewMailEvent { return rec.arrivals() }

// The one thing a new-mail notification must never do is go off for mail that
// was already sitting there when the app started.
func TestTheFirstPassAnnouncesNothing(t *testing.T) {
	svc, rec, acct, inbox := newMailFixture(t)
	deliverLocally(t, svc, acct, inbox, 90, "Eski", "Dünkü mesaj", false)

	svc.announceNewMail(acct)

	if got := newMailEvents(rec); len(got) != 0 {
		t.Errorf("the first pass announced %+v", got)
	}
}

func TestMailArrivingAfterTheFirstPassIsAnnounced(t *testing.T) {
	svc, rec, acct, inbox := newMailFixture(t)
	svc.announceNewMail(acct) // establishes the watermark

	deliverLocally(t, svc, acct, inbox, 91, "Zeynep Aydoğan", "Mutabakat", false)
	svc.announceNewMail(acct)

	got := newMailEvents(rec)
	if len(got) != 1 {
		t.Fatalf("announced %+v, want exactly one event", got)
	}
	if got[0].Count != 1 {
		t.Errorf("Count = %d, want 1", got[0].Count)
	}
	if got[0].From != "Zeynep Aydoğan" || got[0].Subject != "Mutabakat" {
		t.Errorf("event = %+v, want the sender and subject", got[0])
	}
}

// The watch loop wakes on flag changes and expunges as well as arrivals, so
// most passes have nothing to say. A notification per pass would make the app
// unusable.
func TestAPassWithNoArrivalsIsSilent(t *testing.T) {
	svc, rec, acct, inbox := newMailFixture(t)
	svc.announceNewMail(acct)
	deliverLocally(t, svc, acct, inbox, 91, "Zeynep", "Mutabakat", false)
	svc.announceNewMail(acct)

	before := len(newMailEvents(rec))
	svc.announceNewMail(acct)
	svc.announceNewMail(acct)

	if after := len(newMailEvents(rec)); after != before {
		t.Errorf("silent passes produced %d more events", after-before)
	}
}

// Mail that arrived already read was read somewhere else. Announcing it would
// send the reader to a message they have already dealt with.
func TestMailThatArrivesAlreadyReadIsNotAnnounced(t *testing.T) {
	svc, rec, acct, inbox := newMailFixture(t)
	svc.announceNewMail(acct)

	deliverLocally(t, svc, acct, inbox, 91, "Telefonda okundu", "Konu", true)
	svc.announceNewMail(acct)

	if got := newMailEvents(rec); len(got) != 0 {
		t.Errorf("announced %+v for a message that arrived read", got)
	}
}

// Several arrivals in one pass are one notification, not several.
func TestSeveralArrivalsInOnePassAreOneEvent(t *testing.T) {
	svc, rec, acct, inbox := newMailFixture(t)
	svc.announceNewMail(acct)

	deliverLocally(t, svc, acct, inbox, 91, "Bir", "Birinci", false)
	deliverLocally(t, svc, acct, inbox, 92, "İki", "İkinci", false)
	deliverLocally(t, svc, acct, inbox, 93, "Üç", "Üçüncü", false)
	svc.announceNewMail(acct)

	got := newMailEvents(rec)
	if len(got) != 1 {
		t.Fatalf("announced %+v, want one event", got)
	}
	if got[0].Count != 3 {
		t.Errorf("Count = %d, want 3", got[0].Count)
	}
	// The newest is the one worth naming: it is what the reader sees at the
	// top of the list when they open the window.
	if got[0].Subject != "Üçüncü" {
		t.Errorf("Subject = %q, want the newest arrival", got[0].Subject)
	}
}

// Windows shows notifications on the lock screen unless told otherwise, and a
// client that argues for privacy should not be the one deciding a stranger at
// the desk gets to read who wrote and about what.
func TestPreviewsCanBeTurnedOff(t *testing.T) {
	svc, rec, acct, inbox := newMailFixture(t)
	svc.liveNotificationPreview = false
	svc.announceNewMail(acct)

	deliverLocally(t, svc, acct, inbox, 91, "Zeynep", "Gizli konu", false)
	svc.announceNewMail(acct)

	got := newMailEvents(rec)
	if len(got) != 1 {
		t.Fatalf("announced %+v, want one event", got)
	}
	if got[0].From != "" || got[0].Subject != "" {
		t.Errorf("event = %+v, want no content when previews are off", got[0])
	}
	// The count still has to get through, or turning previews off turns
	// notifications off.
	if got[0].Count != 1 {
		t.Errorf("Count = %d with previews off, want 1", got[0].Count)
	}
}

// An account whose first sync has not finished has no inbox. That is a normal
// state, not an error, and it must not panic on the way through.
func TestAnnouncingBeforeTheFirstSyncDoesNothing(t *testing.T) {
	svc, rec, _ := newTestService(t, stubBackend{})

	svc.announceNewMail(model.Account{ID: 999, Email: "yok@example.com"})

	if got := newMailEvents(rec); len(got) != 0 {
		t.Errorf("announced %+v for an account with no inbox", got)
	}
}

func TestTheNotificationReadsLikeAMailNotification(t *testing.T) {
	one := NewMailEvent{Count: 1, Email: "u@example.com", From: "Zeynep Aydoğan", Subject: "Mutabakat"}
	if got := NotificationTitle(one); got != "Zeynep Aydoğan" {
		t.Errorf("title = %q, want the sender", got)
	}
	if got := NotificationBody(one); got != "Mutabakat" {
		t.Errorf("body = %q, want the subject", got)
	}

	many := NewMailEvent{Count: 4, Email: "u@example.com", From: "Zeynep", Subject: "Mutabakat"}
	if got := NotificationBody(many); !strings.Contains(got, "3 more") {
		t.Errorf("body = %q, want the other three mentioned", got)
	}

	// A message with no subject still needs a second line, or the notification
	// is a name and nothing else.
	noSubject := NewMailEvent{Count: 1, From: "Zeynep", Email: "u@example.com"}
	if got := NotificationBody(noSubject); got != "(no subject)" {
		t.Errorf("body = %q for a subjectless message", got)
	}
}

// With previews off the notification has to say something, and the only thing
// left to say is which account and how many.
func TestTheNotificationWithoutPreviewsStillSaysSomething(t *testing.T) {
	e := NewMailEvent{Count: 3, Email: "u@example.com"}

	if got := NotificationTitle(e); got != "3 new messages" {
		t.Errorf("title = %q, want the count", got)
	}
	if got := NotificationBody(e); got != "u@example.com" {
		t.Errorf("body = %q, want the account it arrived in", got)
	}
	if strings.Contains(NotificationTitle(e)+NotificationBody(e), "subject") {
		t.Error("the no-preview notification leaked a placeholder for content")
	}
}
