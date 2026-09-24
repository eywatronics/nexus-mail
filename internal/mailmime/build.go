// Package mailmime assembles an outgoing message.
//
// A pure transformation, like mailhtml and for the same reason: what comes out
// is decided entirely by what goes in, so it can be read and tested without a
// server, a database or a window. Nothing here reaches the network — sending
// is smtpx's job, and what to send is the queue's.
package mailmime

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"
)

// Address is a recipient or sender. The stdlib type, because go-message's is
// an alias for it and there is no reason to invent a third.
type Address = mail.Address

// Attachment is one file travelling with the message.
type Attachment struct {
	Filename string
	// MIMEType may be empty, in which case the message declares the generic
	// binary type rather than guessing. A wrong type is worse than a vague
	// one: it tells the reader's client to open the file with the wrong thing.
	MIMEType string
	Content  []byte
}

// Draft is a message as the writer left it.
type Draft struct {
	From Address
	To   []Address
	Cc   []Address
	// Bcc reaches the server in the envelope and appears in no header. See
	// Recipients, and the test that holds this down.
	Bcc []Address

	Subject string
	// Text is the plain-text body and is always written. A message with an
	// HTML part and no text alternative is one that reads as a blank page in
	// anything that will not render HTML — including this client's own plain
	// text mode.
	Text string
	// HTML is optional. When present the message becomes multipart/alternative.
	HTML string

	Attachments []Attachment

	// InReplyTo is the Message-ID of the message being answered, without
	// angle brackets. Empty for a new message.
	InReplyTo string
	// References is the parent's References plus its Message-ID, which is what
	// threads a conversation in every other client.
	References []string

	// Date is when the message says it was written. Zero means now.
	Date time.Time
	// MessageID overrides the generated one. Only tests set it; a message
	// needs an identifier nobody else will produce.
	MessageID string
}

// Recipients is every address the message has to be delivered to.
//
// This is the only place Bcc appears, and that is the whole point: the
// envelope carries it, the message does not. A client that took its recipients
// from the To and Cc headers would silently fail to deliver blind copies; one
// that wrote Bcc into the headers would show every recipient who else got it.
func (d Draft) Recipients() []string {
	seen := map[string]bool{}
	var out []string
	for _, group := range [][]Address{d.To, d.Cc, d.Bcc} {
		for _, a := range group {
			if a.Address == "" || seen[a.Address] {
				continue
			}
			seen[a.Address] = true
			out = append(out, a.Address)
		}
	}
	return out
}

// Build assembles the RFC 5322 message.
//
// The structure is the smallest one that carries the content: a bare
// text/plain when that is all there is, multipart/alternative when there is
// HTML too, and multipart/mixed around either when files come along.
//
// Built by hand rather than with go-message's mail.CreateWriter, which always
// emits multipart/mixed wrapping multipart/alternative — two wrappers around a
// plain-text note with nothing else in it. The parts are the same either way,
// so a reader flattens them and a round-trip test cannot tell the difference;
// what differs is what an old or minimal client makes of a message that is
// multipart for no reason.
func Build(d Draft) ([]byte, error) {
	if d.From.Address == "" {
		return nil, fmt.Errorf("mailmime: a message needs a sender")
	}
	if len(d.Recipients()) == 0 {
		return nil, fmt.Errorf("mailmime: a message needs at least one recipient")
	}
	for _, att := range d.Attachments {
		if att.Filename == "" {
			return nil, fmt.Errorf("mailmime: an attachment needs a filename")
		}
	}

	header, err := buildHeader(d)
	if err != nil {
		return nil, err
	}

	switch {
	case len(d.Attachments) > 0:
		return buildMixed(d, header)
	case d.HTML != "":
		return buildAlternative(d, header)
	default:
		return buildPlain(d, header)
	}
}

// buildPlain is a message that is one text/plain entity and nothing else.
func buildPlain(d Draft, header gomail.Header) ([]byte, error) {
	setTextPart(&header.Header, "text/plain")

	var buf bytes.Buffer
	w, err := message.CreateWriter(&buf, header.Header)
	if err != nil {
		return nil, fmt.Errorf("mailmime: starting the message: %w", err)
	}
	if _, err := io.WriteString(w, normaliseNewlines(d.Text)); err != nil {
		return nil, fmt.Errorf("mailmime: writing the body: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("mailmime: finishing the message: %w", err)
	}
	return buf.Bytes(), nil
}

// buildAlternative is text and HTML, with no files.
func buildAlternative(d Draft, header gomail.Header) ([]byte, error) {
	header.Set("Content-Type", "multipart/alternative")

	var buf bytes.Buffer
	w, err := message.CreateWriter(&buf, header.Header)
	if err != nil {
		return nil, fmt.Errorf("mailmime: starting the message: %w", err)
	}
	if err := writeAlternativeParts(w, d); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("mailmime: finishing the message: %w", err)
	}
	return buf.Bytes(), nil
}

// buildMixed wraps the body — alternative or plain — beside the files.
func buildMixed(d Draft, header gomail.Header) ([]byte, error) {
	header.Set("Content-Type", "multipart/mixed")

	var buf bytes.Buffer
	w, err := message.CreateWriter(&buf, header.Header)
	if err != nil {
		return nil, fmt.Errorf("mailmime: starting the message: %w", err)
	}

	if d.HTML != "" {
		var inner message.Header
		inner.Set("Content-Type", "multipart/alternative")
		alt, err := w.CreatePart(inner)
		if err != nil {
			return nil, fmt.Errorf("mailmime: starting the body: %w", err)
		}
		if err := writeAlternativeParts(alt, d); err != nil {
			return nil, err
		}
		if err := alt.Close(); err != nil {
			return nil, fmt.Errorf("mailmime: finishing the body: %w", err)
		}
	} else if err := writePart(w, "text/plain", d.Text); err != nil {
		return nil, err
	}

	for _, att := range d.Attachments {
		if err := writeAttachment(w, att); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("mailmime: finishing the message: %w", err)
	}
	return buf.Bytes(), nil
}

// writeAlternativeParts writes plain text then HTML.
//
// Least faithful first. In multipart/alternative a reader takes the last part
// it understands, so this ordering is what makes an HTML-capable client show
// the HTML and a plain one show the text — and it is what every other client
// relies on.
func writeAlternativeParts(w *message.Writer, d Draft) error {
	if err := writePart(w, "text/plain", d.Text); err != nil {
		return err
	}
	return writePart(w, "text/html", d.HTML)
}

func buildHeader(d Draft) (gomail.Header, error) {
	var h gomail.Header

	when := d.Date
	if when.IsZero() {
		when = time.Now()
	}
	h.SetDate(when)

	h.SetAddressList("From", pointers([]Address{d.From}))
	if len(d.To) > 0 {
		h.SetAddressList("To", pointers(d.To))
	}
	if len(d.Cc) > 0 {
		h.SetAddressList("Cc", pointers(d.Cc))
	}
	// No Bcc. Deliberately, and see Recipients.

	h.SetSubject(d.Subject)

	id := d.MessageID
	if id == "" {
		generated, err := newMessageID(d.From.Address)
		if err != nil {
			return h, err
		}
		id = generated
	}
	h.SetMessageID(id)

	if d.InReplyTo != "" {
		h.SetMsgIDList("In-Reply-To", []string{d.InReplyTo})
	}
	if len(d.References) > 0 {
		h.SetMsgIDList("References", d.References)
	}

	// MIME-Version is required of any message that uses MIME at all, and
	// go-message does not add it for the single-part case.
	h.Set("MIME-Version", "1.0")

	// No X-Mailer and no User-Agent.
	//
	// Almost every client announces itself here, and the header travels with
	// the message forever. It is a fingerprint: it names the software, usually
	// the version, and by implication the platform. A client that argues
	// against trackers should not be the one adding an identifier to every
	// message its user sends.
	return h, nil
}

// setTextPart puts the content headers a body part needs on a header.
//
// Quoted-printable rather than 8-bit, always. It makes every message this
// client produces 7-bit clean, which means it can be submitted to a server
// with no 8BITMIME — and the guard in smtpx that refuses 8-bit content on such
// a server becomes a safety net rather than something the normal path trips
// over. The cost is a few percent of size on text that is mostly ASCII.
func setTextPart(h *message.Header, mimeType string) {
	h.Set("Content-Type", mime.FormatMediaType(mimeType, map[string]string{"charset": "utf-8"}))
	h.Set("Content-Transfer-Encoding", "quoted-printable")
}

func writePart(w *message.Writer, mimeType, content string) error {
	var h message.Header
	setTextPart(&h, mimeType)
	h.Set("Content-Disposition", "inline")

	part, err := w.CreatePart(h)
	if err != nil {
		return fmt.Errorf("mailmime: starting the %s part: %w", mimeType, err)
	}
	if _, err := io.WriteString(part, normaliseNewlines(content)); err != nil {
		_ = part.Close()
		return fmt.Errorf("mailmime: writing the %s part: %w", mimeType, err)
	}
	if err := part.Close(); err != nil {
		return fmt.Errorf("mailmime: finishing the %s part: %w", mimeType, err)
	}
	return nil
}

func writeAttachment(w *message.Writer, att Attachment) error {
	mimeType := att.MIMEType
	if mimeType == "" {
		// Not guessed from the extension. A wrong type is worse than a vague
		// one: it tells the reader's client to open the file with the wrong
		// program, and the generic type at least leaves the choice to them.
		mimeType = "application/octet-stream"
	}

	var h message.Header
	h.Set("Content-Type", mimeType)
	h.Set("Content-Transfer-Encoding", "base64")
	h.Set("Content-Disposition",
		mime.FormatMediaType("attachment", map[string]string{"filename": att.Filename}))

	part, err := w.CreatePart(h)
	if err != nil {
		return fmt.Errorf("mailmime: starting the attachment: %w", err)
	}
	if _, err := part.Write(att.Content); err != nil {
		_ = part.Close()
		return fmt.Errorf("mailmime: writing the attachment: %w", err)
	}
	if err := part.Close(); err != nil {
		return fmt.Errorf("mailmime: finishing the attachment: %w", err)
	}
	return nil
}

// newMessageID builds an identifier nobody else will produce.
//
// The domain comes from the sender's address rather than from the machine,
// which is what the obvious implementations do. A hostname is frequently a
// person's name and their employer, and a Message-ID is quoted back in every
// reply and every References header down the thread — so the leak is not to
// one server, it is to everybody who ever sees the conversation.
//
// The local part is random rather than a counter or a timestamp: a guessable
// identifier says how many messages the account has sent and when.
func newMessageID(from string) (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("mailmime: generating a message id: %w", err)
	}

	domain := "localhost"
	if at := strings.LastIndex(from, "@"); at >= 0 && at+1 < len(from) {
		domain = from[at+1:]
	}
	return hex.EncodeToString(buf[:]) + "@" + domain, nil
}

// normaliseNewlines makes the body CRLF.
//
// RFC 5322 defines a line ending as CRLF and SMTP's dot-stuffing works on it.
// A body carrying bare LF survives most of the way and then arrives with the
// last line joined to the terminator on some servers, which is the kind of
// bug that only shows up against one recipient's provider.
func normaliseNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

func pointers(addrs []Address) []*Address {
	out := make([]*Address, 0, len(addrs))
	for i := range addrs {
		out = append(out, &addrs[i])
	}
	return out
}
