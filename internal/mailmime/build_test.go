package mailmime

import (
	"bytes"
	"io"
	"mime"
	"strings"
	"testing"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

func addr(name, address string) Address { return Address{Name: name, Address: address} }

func build(t *testing.T, d Draft) []byte {
	t.Helper()
	raw, err := Build(d)
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	return raw
}

// read parses the built message back, so the assertions are about what a
// receiving client sees rather than about a string this package produced.
func read(t *testing.T, raw []byte) *gomail.Reader {
	t.Helper()
	r, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("the built message does not parse: %v\n%s", err, raw)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

// parts returns every part's content type and decoded body.
func parts(t *testing.T, r *gomail.Reader) map[string]string {
	t.Helper()

	out := map[string]string{}
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("reading a part: %v", err)
		}
		body, err := io.ReadAll(p.Body)
		if err != nil {
			t.Fatalf("reading a part body: %v", err)
		}
		ct, _, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if err != nil {
			t.Fatalf("a part declares an unparseable content type %q: %v",
				p.Header.Get("Content-Type"), err)
		}
		out[ct] = string(body)
	}
}

func simple() Draft {
	return Draft{
		From:    addr("Yazan", "u@example.com"),
		To:      []Address{addr("Okuyan", "r@example.com")},
		Subject: "Konu",
		Text:    "metin",
	}
}

// The classic mail bug, and the reason Recipients exists.
func TestBccReachesTheEnvelopeAndNeverTheMessage(t *testing.T) {
	d := simple()
	d.Cc = []Address{addr("", "c@example.com")}
	d.Bcc = []Address{addr("Gizli", "hidden@example.com")}

	raw := build(t, d)

	if bytes.Contains(raw, []byte("hidden@example.com")) {
		t.Errorf("the blind copy is in the message:\n%s", raw)
	}
	if bytes.Contains(bytes.ToLower(raw), []byte("bcc:")) {
		t.Error("a Bcc header was written")
	}

	got := d.Recipients()
	want := []string{"r@example.com", "c@example.com", "hidden@example.com"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Recipients() = %v, want %v", got, want)
	}
}

// One person on both To and Cc is one delivery, not two.
func TestRecipientsAreDeduplicated(t *testing.T) {
	d := simple()
	d.Cc = []Address{addr("", "r@example.com"), addr("", "c@example.com")}
	d.Bcc = []Address{addr("", "c@example.com")}

	if got := d.Recipients(); len(got) != 2 {
		t.Errorf("Recipients() = %v, want two distinct addresses", got)
	}
}

func TestBuildRefusesAMessageThatCannotBeSent(t *testing.T) {
	noSender := simple()
	noSender.From = Address{}
	if _, err := Build(noSender); err == nil {
		t.Error("Build() accepted a message with no sender")
	}

	noRecipients := simple()
	noRecipients.To = nil
	if _, err := Build(noRecipients); err == nil {
		t.Error("Build() accepted a message with nobody to send it to")
	}
}

// A message with only text must not be multipart at all.
//
// Asserted on the raw bytes rather than through the reader. The reader
// flattens nesting, so it returns one text/plain part whether the message is a
// bare entity or two wrappers deep — which is exactly how two pointless
// wrappers went unnoticed here until the message was read by eye.
func TestATextOnlyMessageIsASinglePart(t *testing.T) {
	raw := build(t, simple())

	if bytes.Contains(bytes.ToLower(raw), []byte("multipart")) {
		t.Errorf("a text-only message is wrapped in multipart:\n%s", raw)
	}
	if header := topHeader(t, raw); !strings.Contains(header, "text/plain") {
		t.Errorf("the top-level type is not text/plain:\n%s", header)
	}

	got := parts(t, read(t, raw))
	if body, ok := got["text/plain"]; !ok || !strings.Contains(body, "metin") {
		t.Errorf("the text part is missing or wrong: %v", got)
	}
}

// topHeader is everything before the blank line that ends the headers.
func topHeader(t *testing.T, raw []byte) string {
	t.Helper()
	head, _, found := bytes.Cut(raw, []byte("\r\n\r\n"))
	if !found {
		t.Fatalf("the message has no header block:\n%s", raw)
	}
	return string(head)
}

// Least faithful first, so a reader taking the last part it understands gets
// the HTML. That ordering is what every other client relies on.
func TestHTMLBecomesAnAlternativeWithPlainTextFirst(t *testing.T) {
	d := simple()
	d.HTML = "<p>metin</p>"

	raw := build(t, d)
	got := parts(t, read(t, raw))

	if len(got) != 2 {
		t.Fatalf("expected two parts, got %v", got)
	}
	if !strings.Contains(got["text/plain"], "metin") {
		t.Error("the plain-text alternative is missing")
	}
	if !strings.Contains(got["text/html"], "<p>metin</p>") {
		t.Error("the HTML alternative is missing")
	}

	header := topHeader(t, raw)
	if !strings.Contains(header, "multipart/alternative") {
		t.Errorf("the message is not multipart/alternative:\n%s", header)
	}
	// And not wrapped in a mixed part it has no files to justify.
	if strings.Contains(header, "multipart/mixed") {
		t.Errorf("text and HTML alone were wrapped in multipart/mixed:\n%s", header)
	}
	if strings.Index(string(raw), "text/plain") > strings.Index(string(raw), "text/html") {
		t.Error("the HTML part comes before the plain-text one")
	}
}

// Quoted-printable everywhere means every message this client sends is 7-bit
// clean, so a server with no 8BITMIME takes it without argument.
func TestTheWholeMessageIsSevenBitClean(t *testing.T) {
	d := simple()
	d.Subject = "Çağrı: ışık"
	d.Text = "Gövde ışıkla dolu — ağ, ş, ı, İ."
	d.HTML = "<p>Gövde ışıkla dolu</p>"

	raw := build(t, d)

	for i, b := range raw {
		if b > 127 {
			t.Fatalf("byte %d is 0x%02x; the message is not 7-bit clean", i, b)
		}
	}

	// And it still decodes back to what was written.
	got := parts(t, read(t, raw))
	if !strings.Contains(got["text/plain"], "ışıkla") {
		t.Errorf("the text did not survive the encoding: %q", got["text/plain"])
	}
}

// A Turkish subject has to arrive as a Turkish subject, which means RFC 2047
// on the wire and the original after decoding.
func TestANonASCIISubjectIsEncodedAndComesBack(t *testing.T) {
	d := simple()
	d.Subject = "Şubat toplantısı"

	raw := build(t, d)

	if bytes.Contains(raw, []byte("Şubat")) {
		t.Error("the subject was written as raw UTF-8 rather than encoded")
	}

	subject, err := read(t, raw).Header.Subject()
	if err != nil {
		t.Fatalf("reading the subject: %v", err)
	}
	if subject != "Şubat toplantısı" {
		t.Errorf("Subject = %q, want the original", subject)
	}
}

func TestADisplayNameSurvivesTheRoundTrip(t *testing.T) {
	d := simple()
	d.From = addr("İsmet Kabatepe", "u@example.com")

	list, err := read(t, build(t, d)).Header.AddressList("From")
	if err != nil {
		t.Fatalf("reading From: %v", err)
	}
	if len(list) != 1 || list[0].Name != "İsmet Kabatepe" {
		t.Errorf("From = %v, want the original display name", list)
	}
}

func TestAnAttachmentTravelsBase64WithItsName(t *testing.T) {
	d := simple()
	d.Attachments = []Attachment{{
		Filename: "rapor.pdf",
		MIMEType: "application/pdf",
		Content:  []byte{0x25, 0x50, 0x44, 0x46, 0xFF, 0x00, 0xFE},
	}}

	raw := build(t, d)

	header := topHeader(t, raw)
	if !strings.Contains(header, "multipart/mixed") {
		t.Errorf("a message with a file is not multipart/mixed:\n%s", header)
	}

	r := read(t, raw)
	var found bool
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("reading a part: %v", err)
		}
		att, ok := p.Header.(*gomail.AttachmentHeader)
		if !ok {
			continue
		}
		name, err := att.Filename()
		if err != nil {
			t.Fatalf("reading the filename: %v", err)
		}
		if name != "rapor.pdf" {
			t.Errorf("filename = %q", name)
		}
		body, _ := io.ReadAll(p.Body)
		if !bytes.Equal(body, d.Attachments[0].Content) {
			t.Errorf("the file came back as % x", body)
		}
		found = true
	}
	if !found {
		t.Error("no attachment part was written")
	}
}

// A wrong type tells the reader's client to open the file with the wrong
// program. The vague one at least leaves the choice to them.
func TestAnAttachmentWithNoTypeIsGenericRatherThanGuessed(t *testing.T) {
	d := simple()
	d.Attachments = []Attachment{{Filename: "belge.pdf", Content: []byte("x")}}

	raw := build(t, d)
	if !bytes.Contains(raw, []byte("application/octet-stream")) {
		t.Error("an untyped attachment was not declared as generic binary")
	}
	if bytes.Contains(raw, []byte("application/pdf")) {
		t.Error("the type was guessed from the extension")
	}
}

func TestAnAttachmentNeedsAName(t *testing.T) {
	d := simple()
	d.Attachments = []Attachment{{Content: []byte("x")}}

	if _, err := Build(d); err == nil {
		t.Error("Build() accepted an attachment with no filename")
	}
}

// The Message-ID is quoted back in every reply and in the References of every
// message down the thread. A hostname there is a leak to everybody who ever
// sees the conversation, not just to one server.
func TestTheMessageIDCarriesTheSendersDomainNotTheMachines(t *testing.T) {
	raw := build(t, simple())

	id, err := read(t, raw).Header.MessageID()
	if err != nil {
		t.Fatalf("reading the message id: %v", err)
	}
	if !strings.HasSuffix(id, "@example.com") {
		t.Errorf("Message-ID = %q; the domain is not the sender's", id)
	}
	if strings.Contains(id, "localhost") {
		t.Errorf("Message-ID = %q; it fell back to a placeholder domain", id)
	}
}

// A guessable identifier says how many messages the account has sent and when.
func TestTwoMessagesGetDifferentIdentifiers(t *testing.T) {
	first, _ := read(t, build(t, simple())).Header.MessageID()
	second, _ := read(t, build(t, simple())).Header.MessageID()

	if first == second {
		t.Errorf("both messages got the id %q", first)
	}
}

// Almost every client announces itself here, and the header travels with the
// message forever.
func TestTheMessageDoesNotNameTheSoftwareThatWroteIt(t *testing.T) {
	raw := bytes.ToLower(build(t, simple()))

	for _, header := range []string{"x-mailer:", "user-agent:", "nexus"} {
		if bytes.Contains(raw, []byte(header)) {
			t.Errorf("the message carries %q", header)
		}
	}
}

// RFC 5322 defines a line ending as CRLF and SMTP's dot-stuffing works on it.
// A bare LF survives most of the way and then breaks against one provider.
func TestEveryLineEndsWithCRLF(t *testing.T) {
	d := simple()
	d.Text = "birinci satır\nikinci satır\r\nüçüncü satır"

	raw := build(t, d)

	for i := range raw {
		if raw[i] == '\n' && (i == 0 || raw[i-1] != '\r') {
			t.Fatalf("a bare LF at byte %d:\n%s", i, raw)
		}
	}
}

// Threading is what makes a reply appear under what it answers.
func TestAReplyCarriesTheThreadingHeaders(t *testing.T) {
	d := simple()
	d.InReplyTo = "parent@example.com"
	d.References = []string{"root@example.com", "parent@example.com"}

	header := read(t, build(t, d)).Header

	inReplyTo, err := header.MsgIDList("In-Reply-To")
	if err != nil {
		t.Fatalf("reading In-Reply-To: %v", err)
	}
	if len(inReplyTo) != 1 || inReplyTo[0] != "parent@example.com" {
		t.Errorf("In-Reply-To = %v", inReplyTo)
	}

	refs, err := header.MsgIDList("References")
	if err != nil {
		t.Fatalf("reading References: %v", err)
	}
	if len(refs) != 2 || refs[0] != "root@example.com" {
		t.Errorf("References = %v", refs)
	}
}

func TestANewMessageHasNoThreadingHeaders(t *testing.T) {
	raw := bytes.ToLower(build(t, simple()))

	for _, header := range []string{"in-reply-to:", "references:"} {
		if bytes.Contains(raw, []byte(header)) {
			t.Errorf("a new message carries %q", header)
		}
	}
}

func TestTheDateIsWrittenAndReadBack(t *testing.T) {
	d := simple()
	d.Date = time.Date(2026, 9, 24, 10, 30, 0, 0, time.FixedZone("+03", 3*3600))

	got, err := read(t, build(t, d)).Header.Date()
	if err != nil {
		t.Fatalf("reading the date: %v", err)
	}
	if !got.Equal(d.Date) {
		t.Errorf("Date = %s, want %s", got, d.Date)
	}
}

// Required of any message that uses MIME, and go-message does not add it for
// the single-part case.
func TestMIMEVersionIsDeclared(t *testing.T) {
	if got := read(t, build(t, simple())).Header.Get("MIME-Version"); got != "1.0" {
		t.Errorf("MIME-Version = %q", got)
	}
}

func TestTheTextPartDeclaresUTF8(t *testing.T) {
	raw := build(t, simple())

	r := read(t, raw)
	p, err := r.NextPart()
	if err != nil {
		t.Fatalf("reading the first part: %v", err)
	}
	ct, params, err := mime.ParseMediaType(p.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("reading the content type: %v", err)
	}
	if !strings.EqualFold(params["charset"], "utf-8") {
		t.Errorf("charset = %q", params["charset"])
	}
	if ct != "text/plain" {
		t.Errorf("the first part is %q, want text/plain", ct)
	}
}
