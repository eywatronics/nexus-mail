package app

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"nexusmail/internal/mailmime"
	"nexusmail/internal/model"
)

// DraftDTO is a message as the window hands it over.
//
// Addresses arrive as text because that is what a person types. Parsing them
// here rather than in the window means one implementation of "what counts as
// an address", and it means the window cannot hand the backend something the
// backend then has to guess about.
type DraftDTO struct {
	// IdentityID is who the message is from. Zero means the account's default,
	// which is what a composer opened from the message list uses.
	IdentityID int64 `json:"identityId"`
	AccountID  int64 `json:"accountId"`

	To  string `json:"to"`
	Cc  string `json:"cc"`
	Bcc string `json:"bcc"`

	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`

	// InReplyTo and References thread the message under what it answers.
	InReplyTo  string   `json:"inReplyTo"`
	References []string `json:"references"`

	// AttachmentPaths are files the person chose in the dialog, by path. Only
	// paths PickAttachments handed out are accepted; see readAttachments.
	AttachmentPaths []string `json:"attachmentPaths"`

	// DraftID is the saved draft this message came from, zero when it never
	// was one. Cleared once the message is queued: a draft left behind after
	// the message went out is a message the user will send a second time.
	DraftID int64 `json:"draftId"`
}

// QueuedDTO is what the window is told about a message on its way.
type QueuedDTO struct {
	OperationID int64 `json:"operationId"`
	// Recipients is how many addresses it will be delivered to, which is not
	// the number of addresses on screen: a blind copy is a recipient and a
	// duplicate across To and Cc is not a second one.
	Recipients int `json:"recipients"`
}

// SendMessage assembles a message and queues it.
//
// Queued, never sent here. The window gets an answer in the time it takes to
// write a file, and the message goes out when the connection allows — which on
// a train is later and on a working network is a second afterwards. Sending
// synchronously would mean a composer that hangs for the length of a TLS
// handshake and an upload, and a message lost if the user closed it.
func (s *MailService) SendMessage(d DraftDTO) (QueuedDTO, error) {
	ctx := context.Background()

	if s.outbox == nil {
		return QueuedDTO{}, fmt.Errorf("app: this build has nowhere to queue outgoing mail")
	}

	identity, err := s.identityFor(ctx, d)
	if err != nil {
		return QueuedDTO{}, err
	}

	// Read before the draft is assembled, so a file that has gone missing
	// since it was chosen stops the send rather than producing a message with
	// a paperclip the reader cannot open.
	attachments, err := s.readAttachments(d.AttachmentPaths)
	if err != nil {
		return QueuedDTO{}, err
	}

	draft, err := draftFromDTO(d, identity, attachments)
	if err != nil {
		return QueuedDTO{}, err
	}

	raw, err := mailmime.Build(draft)
	if err != nil {
		return QueuedDTO{}, err
	}

	// The file first, then the queue row. The other order would leave a row
	// naming a file that does not exist, which the worker can only resolve by
	// failing the message permanently. A file with no row is swept later and
	// costs nothing but disk.
	name, err := s.outbox.Put(raw)
	if err != nil {
		return QueuedDTO{}, err
	}

	op := model.Operation{
		AccountID:    identity.AccountID,
		Kind:         model.OpSend,
		Outbox:       name,
		EnvelopeFrom: identity.Email,
		EnvelopeTo:   draft.Recipients(),
	}
	id, err := s.store.EnqueueOperation(ctx, op)
	if err != nil {
		// The row is what makes the file mean anything. Without it the message
		// would sit on disk forever, so it goes now rather than waiting for a
		// sweep to notice.
		_ = s.outbox.Remove(name)
		return QueuedDTO{}, err
	}

	// After the queue row, never before. A draft discarded first and a queue
	// row that then failed to write would be a message that exists nowhere.
	if d.DraftID != 0 {
		// Ignored on purpose. The message is queued and will go out; reporting
		// this as a failed send would invite a second copy of it. The cost is
		// a draft that lingers, and that is not a silent failure — it is
		// sitting in the Drafts list where the person can see it and throw it
		// away, which is more than a log line would give them.
		_ = s.store.DeleteDraft(ctx, d.DraftID)
	}

	s.nudgeWatchers()
	return QueuedDTO{OperationID: id, Recipients: len(op.EnvelopeTo)}, nil
}

// identityFor resolves who the message is from.
func (s *MailService) identityFor(ctx context.Context, d DraftDTO) (model.Identity, error) {
	if d.IdentityID != 0 {
		list, err := s.store.ListIdentities(ctx, d.AccountID)
		if err != nil {
			return model.Identity{}, err
		}
		for _, i := range list {
			if i.ID == d.IdentityID {
				return i, nil
			}
		}
		// Named but not found, which is not the same as not named. Falling
		// back to the default would send the message as somebody the writer
		// did not choose.
		return model.Identity{}, fmt.Errorf(
			"app: identity %d does not belong to account %d", d.IdentityID, d.AccountID)
	}
	if d.AccountID == 0 {
		return model.Identity{}, fmt.Errorf("app: a message needs an account to be sent from")
	}
	return s.store.DefaultIdentity(ctx, d.AccountID)
}

// draftFromDTO turns what the window sent into what the builder takes.
func draftFromDTO(d DraftDTO, identity model.Identity,
	attachments []mailmime.Attachment) (mailmime.Draft, error) {
	to, err := parseAddressList("To", d.To)
	if err != nil {
		return mailmime.Draft{}, err
	}
	cc, err := parseAddressList("Cc", d.Cc)
	if err != nil {
		return mailmime.Draft{}, err
	}
	bcc, err := parseAddressList("Bcc", d.Bcc)
	if err != nil {
		return mailmime.Draft{}, err
	}
	// The identity's, not the window's. Where answers should go is a property
	// of the address being written from — a support alias whose replies belong
	// in a shared mailbox wants that on every message, not on the ones the
	// writer remembered to set it on.
	replyTo, err := parseAddressList("Reply-To", identity.ReplyTo)
	if err != nil {
		return mailmime.Draft{}, err
	}

	text := withSignature(d.Text, identity.SignatureText)
	html := d.HTML
	if html != "" {
		html = withHTMLSignature(html, identity.SignatureHTML, identity.SignatureText)
	}

	return mailmime.Draft{
		From:        mailmime.Address{Name: identity.DisplayName, Address: identity.Email},
		To:          to,
		Cc:          cc,
		Bcc:         bcc,
		ReplyTo:     replyTo,
		Attachments: attachments,
		Subject:     d.Subject,
		Text:        text,
		HTML:        html,
		InReplyTo:   d.InReplyTo,
		References:  d.References,
	}, nil
}

// parseAddressList reads one header's worth of addresses.
//
// An unparseable address is refused rather than dropped. Silently sending to
// three of four recipients is the kind of failure nobody notices until the
// fourth person asks why they were left out.
func parseAddressList(field, value string) ([]mailmime.Address, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		return nil, fmt.Errorf("app: the %s line is not a list of addresses: %w", field, err)
	}

	out := make([]mailmime.Address, 0, len(parsed))
	for _, a := range parsed {
		out = append(out, *a)
	}
	return out, nil
}

// signatureSeparator is the two-dash line RFC 3676 defines.
//
// It is what tells a receiving client where the message ends and the signature
// begins, which is how a reply quotes the one and not the other. Writing the
// signature without it produces a reply that quotes somebody's job title back
// at them every time.
const signatureSeparator = "-- \n"

func withSignature(body, signature string) string {
	if strings.TrimSpace(signature) == "" {
		return body
	}
	return strings.TrimRight(body, "\n") + "\n\n" + signatureSeparator + signature
}

// withHTMLSignature appends the HTML signature, falling back to the text one.
//
// A signature that exists only as text still belongs on an HTML message: the
// alternative is a message whose two halves disagree about who sent it. The
// text is escaped rather than inserted, because a signature is content and a
// name with an ampersand in it is not markup.
func withHTMLSignature(body, htmlSignature, textSignature string) string {
	if strings.TrimSpace(htmlSignature) != "" {
		return body + "\n<div class=\"nx-signature\">" + htmlSignature + "</div>\n"
	}
	if strings.TrimSpace(textSignature) == "" {
		return body
	}
	return body + "\n<div class=\"nx-signature\"><pre>" +
		htmlEscape(textSignature) + "</pre></div>\n"
}

// IdentityDTO is one address an account can send as.
type IdentityDTO struct {
	ID          int64  `json:"id"`
	AccountID   int64  `json:"accountId"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	// From is the two together, as they appear in a From header, so the
	// composer shows what the recipient will see rather than assembling it a
	// second time and getting the quoting subtly different.
	From      string `json:"from"`
	IsDefault bool   `json:"isDefault"`
}

// Identities lists the addresses an account can send as, default first.
func (s *MailService) Identities(accountID int64) ([]IdentityDTO, error) {
	list, err := s.store.ListIdentities(context.Background(), accountID)
	if err != nil {
		return nil, err
	}

	out := make([]IdentityDTO, 0, len(list))
	for _, i := range list {
		out = append(out, IdentityDTO{
			ID: i.ID, AccountID: i.AccountID,
			Email: i.Email, DisplayName: i.DisplayName,
			From: i.From(), IsDefault: i.IsDefault,
		})
	}
	return out, nil
}
