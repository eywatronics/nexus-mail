package app

import (
	"context"
	"fmt"
	"strings"

	"nexusmail/internal/mailmime"
	"nexusmail/internal/model"
)

// ReplyDraftDTO is a composer opened on an existing message.
//
// Assembled here rather than in the window because every part of it needs
// something the window does not have: the Message-ID and References for
// threading, the full address lists, and the body — which the window holds
// only as sanitised HTML inside a sandboxed frame it cannot read back.
type ReplyDraftDTO struct {
	AccountID int64 `json:"accountId"`

	To      string `json:"to"`
	Cc      string `json:"cc"`
	Subject string `json:"subject"`

	InReplyTo  string   `json:"inReplyTo"`
	References []string `json:"references"`

	// Quoted is the original, marked up and ready to sit under the reply.
	Quoted string `json:"quoted"`
}

// ReplyDraft builds the draft for answering a message.
//
// all decides between answering the sender and answering everybody. The two
// are one function because they differ in exactly one thing — who ends up on
// the Cc line — and two functions would be two places to get the exclusion of
// the reader's own addresses wrong.
func (s *MailService) ReplyDraft(messageID int64, all bool) (ReplyDraftDTO, error) {
	ctx := context.Background()

	msg, _, acct, err := s.locateFullMessage(ctx, messageID)
	if err != nil {
		return ReplyDraftDTO{}, err
	}

	// From, and not Reply-To, because Reply-To is not captured yet.
	//
	// That is a real gap and it shows on mailing lists: a list that sets
	// Reply-To expects answers to go to the list, and this sends them to
	// whoever happened to post. Closing it needs the header stored, which
	// needs a column and a line in the envelope parser — its own piece of
	// work rather than a guess made here.
	to := []model.Address{msg.From}

	var cc []model.Address
	if all {
		// Everyone who was already on it, minus us. Without the exclusion the
		// reader mails themselves every time they answer a group — and on a
		// thread of ten replies that is ten copies in their own inbox.
		mine, err := s.ownAddresses(ctx, acct.ID)
		if err != nil {
			return ReplyDraftDTO{}, err
		}
		cc = withoutAddresses(append(append([]model.Address{}, msg.To...), msg.Cc...),
			mine, addressSet(to))
	}

	body, err := s.plainBodyOf(ctx, messageID)
	if err != nil {
		// A body that cannot be read is not a reason to refuse the reply. The
		// reader can still write one; they simply do not get the original
		// quoted under it.
		body = ""
	}

	return ReplyDraftDTO{
		AccountID:  acct.ID,
		To:         formatAddresses(to),
		Cc:         formatAddresses(cc),
		Subject:    mailmime.ReplySubject(msg.Subject),
		InReplyTo:  msg.MessageID,
		References: mailmime.ReplyReferences(msg.References, msg.MessageID),
		Quoted:     mailmime.Quote(msg.From.String(), msg.Date, body),
	}, nil
}

// ForwardDraft builds the draft for passing a message on.
//
// No recipients and no threading. A forward starts a new conversation with
// somebody who was not in the old one, and threading it under the original
// would file it in a thread they have never seen.
func (s *MailService) ForwardDraft(messageID int64) (ReplyDraftDTO, error) {
	ctx := context.Background()

	msg, _, acct, err := s.locateFullMessage(ctx, messageID)
	if err != nil {
		return ReplyDraftDTO{}, err
	}

	body, err := s.plainBodyOf(ctx, messageID)
	if err != nil {
		body = ""
	}

	return ReplyDraftDTO{
		AccountID: acct.ID,
		Subject:   mailmime.ForwardSubject(msg.Subject),
		Quoted:    forwardedHeader(msg) + "\n" + body,
	}, nil
}

// forwardedHeader is the block that says where a forwarded message came from.
//
// Written out rather than quoted with markers: the recipient is being shown a
// message, not a conversation they were part of, and the headers are the part
// they need to judge it.
func forwardedHeader(msg model.Message) string {
	var b strings.Builder
	b.WriteString("---------- Forwarded message ----------\n")
	b.WriteString("From: " + msg.From.String() + "\n")
	if !msg.Date.IsZero() {
		b.WriteString("Date: " + msg.Date.Format("2 January 2006 at 15:04") + "\n")
	}
	b.WriteString("Subject: " + msg.Subject + "\n")
	if len(msg.To) > 0 {
		b.WriteString("To: " + formatAddresses(msg.To) + "\n")
	}
	if len(msg.Cc) > 0 {
		b.WriteString("Cc: " + formatAddresses(msg.Cc) + "\n")
	}
	return b.String()
}

// plainBodyOf returns the message as text, fetching it if it is not cached.
func (s *MailService) plainBodyOf(ctx context.Context, messageID int64) (string, error) {
	msg, folder, acct, err := s.locateMessage(ctx, messageID)
	if err != nil {
		return "", err
	}
	body, err := s.engine.EnsureBody(ctx, acct, folder, msg)
	if err != nil {
		return "", err
	}
	return plainTextOf(body)
}

// ownAddresses is every address this account can send as.
//
// The identities, not the account's one address. Somebody who answers support@
// from their personal mailbox would otherwise find support@ on the Cc line of
// every reply-all they send.
func (s *MailService) ownAddresses(ctx context.Context, accountID int64) (map[string]bool, error) {
	list, err := s.store.ListIdentities(ctx, accountID)
	if err != nil {
		return nil, err
	}

	out := map[string]bool{}
	for _, i := range list {
		out[strings.ToLower(i.Email)] = true
	}
	return out, nil
}

func addressSet(addrs []model.Address) map[string]bool {
	out := map[string]bool{}
	for _, a := range addrs {
		out[strings.ToLower(a.Addr)] = true
	}
	return out
}

// withoutAddresses drops anyone already accounted for, and any duplicate.
func withoutAddresses(addrs []model.Address, exclude ...map[string]bool) []model.Address {
	seen := map[string]bool{}
	var out []model.Address

	for _, a := range addrs {
		key := strings.ToLower(a.Addr)
		if key == "" || seen[key] {
			continue
		}
		skip := false
		for _, set := range exclude {
			if set[key] {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		seen[key] = true
		out = append(out, a)
	}
	return out
}

func formatAddresses(addrs []model.Address) string {
	parts := make([]string, 0, len(addrs))
	for _, a := range addrs {
		parts = append(parts, a.String())
	}
	return strings.Join(parts, ", ")
}

// locateFullMessage reads the whole message row, not just the identifiers
// locateMessage returns.
func (s *MailService) locateFullMessage(ctx context.Context, messageID int64) (
	model.Message, model.Folder, model.Account, error) {

	msgs, err := s.store.MessagesByIDs(ctx, []int64{messageID})
	if err != nil {
		return model.Message{}, model.Folder{}, model.Account{}, err
	}
	if len(msgs) == 0 {
		return model.Message{}, model.Folder{}, model.Account{},
			fmt.Errorf("app: no message with id %d", messageID)
	}

	folder, acct, err := s.locateFolder(ctx, msgs[0].FolderID)
	if err != nil {
		return model.Message{}, model.Folder{}, model.Account{}, err
	}
	return msgs[0], folder, acct, nil
}
