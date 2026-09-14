package app

import (
	"context"
	"fmt"

	"nexusmail/internal/model"
)

// EventNewMail fires when unread mail has arrived in an inbox since the last
// time the reader was told.
const EventNewMail = "mail:new"

// NewMailEvent is what the window and the operating system's notification
// centre are given.
type NewMailEvent struct {
	AccountID int64  `json:"accountId"`
	Email     string `json:"email"`
	// Count is how many unread messages arrived in this pass.
	Count int `json:"count"`
	// From and Subject describe the newest of them, for a notification that
	// says something rather than just a number. Both are empty when the reader
	// has turned previews off.
	From    string `json:"from,omitempty"`
	Subject string `json:"subject,omitempty"`
}

// maxArrivalsPerPass caps how much is read to build one notification. A
// mailbox being backfilled produces one sentence either way.
const maxArrivalsPerPass = 50

// announceNewMail reports unread mail that has arrived in an account's inbox
// since the last announcement.
//
// The watermark is a row id rather than a timestamp. Row ids are monotonic in
// the order this client learned about messages, so a mail that was sent last
// week but only reached us on this pass is announced once, when we find it —
// which is when it is news.
//
// A pass that finds nothing announces nothing. The watch loop wakes on every
// flag change and every expunge as well as on arrivals, so most passes are
// silent and a notification per pass would make the app unusable.
func (s *MailService) announceNewMail(acct model.Account) {
	ctx := context.Background()

	inbox, ok := s.inboxOf(ctx, acct.ID)
	if !ok {
		return
	}

	watermark, known := s.arrivalWatermark(acct.ID)
	if !known {
		// First pass for this account since the app started. Everything
		// already in the mailbox is not news, so the watermark starts at the
		// top and this pass announces nothing.
		highest, err := s.store.HighestMessageID(ctx, inbox.ID)
		if err != nil {
			return
		}
		s.setArrivalWatermark(acct.ID, highest)
		return
	}

	arrived, err := s.store.MessagesAfterID(ctx, inbox.ID, watermark, maxArrivalsPerPass)
	if err != nil || len(arrived) == 0 {
		return
	}

	// The watermark advances over everything seen, read or not. A message the
	// reader had already opened on their phone is not news here either.
	s.setArrivalWatermark(acct.ID, arrived[len(arrived)-1].ID)

	var unread []model.Message
	for _, m := range arrived {
		if !m.HasFlag(model.FlagSeen) {
			unread = append(unread, m)
		}
	}
	if len(unread) == 0 {
		return
	}

	event := NewMailEvent{
		AccountID: acct.ID,
		Email:     acct.Email,
		Count:     len(unread),
	}
	if s.cfg.NotificationPreview {
		newest := unread[len(unread)-1]
		event.From = senderLabel(newest)
		event.Subject = newest.Subject
	}
	s.cfg.Emit(EventNewMail, event)
}

// senderLabel is the name to show, falling back to the address. A message with
// neither is rare and gets a word rather than an empty line.
func senderLabel(m model.Message) string {
	switch {
	case m.From.Name != "":
		return m.From.Name
	case m.From.Addr != "":
		return m.From.Addr
	default:
		return "Unknown sender"
	}
}

// NotificationTitle and NotificationBody format one arrival event for the
// operating system.
//
// Split out and exported because main.go is the only caller and main.go is the
// one file with no tests: keeping the wording here keeps it checkable.
func NotificationTitle(e NewMailEvent) string {
	if e.From != "" {
		return e.From
	}
	if e.Count == 1 {
		return "1 new message"
	}
	return fmt.Sprintf("%d new messages", e.Count)
}

func NotificationBody(e NewMailEvent) string {
	subject := e.Subject
	if subject == "" && e.From != "" {
		subject = "(no subject)"
	}

	switch {
	case subject == "":
		// Previews are off, so the title already carries the count and there
		// is nothing left to say that is not the content being withheld.
		return e.Email
	case e.Count > 1:
		return fmt.Sprintf("%s — and %d more", subject, e.Count-1)
	default:
		return subject
	}
}

// inboxOf finds an account's inbox. An account whose first sync has not
// finished has none yet, which is not an error.
func (s *MailService) inboxOf(ctx context.Context, accountID int64) (model.Folder, bool) {
	folders, err := s.store.ListFolders(ctx, accountID)
	if err != nil {
		return model.Folder{}, false
	}
	for _, f := range folders {
		if f.IsInbox() {
			return f, true
		}
	}
	return model.Folder{}, false
}

func (s *MailService) arrivalWatermark(accountID int64) (int64, bool) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()

	if s.arrivals == nil {
		return 0, false
	}
	mark, ok := s.arrivals[accountID]
	return mark, ok
}

func (s *MailService) setArrivalWatermark(accountID, mark int64) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()

	if s.arrivals == nil {
		s.arrivals = map[int64]int64{}
	}
	s.arrivals[accountID] = mark
}
