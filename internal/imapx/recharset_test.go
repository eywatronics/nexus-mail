package imapx

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// Turkish in ISO-8859-9, byte by byte: "Mutabakat sözleşmesi". Written as
// bytes rather than as a Go string literal on purpose — a literal would be
// UTF-8, which is the one encoding this file is not about.
var latin5Turkish = []byte{
	'M', 'u', 't', 'a', 'b', 'a', 'k', 'a', 't', ' ',
	's', 0xF6, 'z', 'l', 'e', 0xFE, 'm', 'e', 's', 'i',
}

const decodedTurkish = "Mutabakat sözleşmesi"

// rawMessage assembles headers and a body into an on-the-wire message.
func rawMessage(headers []string, body []byte) []byte {
	var b bytes.Buffer
	for _, h := range headers {
		b.WriteString(h + "\r\n")
	}
	b.WriteString("\r\n")
	b.Write(body)
	return b.Bytes()
}

// The declared charset is frequently a lie. A message whose headers say UTF-8
// and whose bytes are Latin-5 renders as mojibake, and nothing in the normal
// path can fix it: the normal path is doing exactly what it was told.
func TestForcingACharsetDecodesAMislabelledMessage(t *testing.T) {
	raw := rawMessage([]string{
		"Subject: Rapor",
		"Content-Type: text/plain; charset=utf-8",
	}, latin5Turkish)

	body, err := DecodeBodyForcingCharset(raw, "iso-8859-9")
	if err != nil {
		t.Fatalf("DecodeBodyForcingCharset() error: %v", err)
	}
	if body.Text != decodedTurkish {
		t.Errorf("Text = %q, want %q", body.Text, decodedTurkish)
	}
}

// The transfer encoding has to be undone before the character set is applied,
// and exactly once. Doing it twice, or in the other order, produces a second
// kind of mojibake on top of the first.
func TestForcingACharsetUndoesQuotedPrintableFirst(t *testing.T) {
	// The same Turkish, quoted-printable over Latin-5 bytes.
	raw := rawMessage([]string{
		"Subject: Rapor",
		"Content-Type: text/plain; charset=us-ascii",
		"Content-Transfer-Encoding: quoted-printable",
	}, []byte("Mutabakat s=F6zle=FEmesi"))

	body, err := DecodeBodyForcingCharset(raw, "iso-8859-9")
	if err != nil {
		t.Fatalf("DecodeBodyForcingCharset() error: %v", err)
	}
	if body.Text != decodedTurkish {
		t.Errorf("Text = %q, want %q", body.Text, decodedTurkish)
	}
}

func TestForcingACharsetUndoesBase64First(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(latin5Turkish)
	raw := rawMessage([]string{
		"Subject: Rapor",
		"Content-Type: text/plain; charset=us-ascii",
		"Content-Transfer-Encoding: base64",
	}, []byte(encoded))

	body, err := DecodeBodyForcingCharset(raw, "iso-8859-9")
	if err != nil {
		t.Fatalf("DecodeBodyForcingCharset() error: %v", err)
	}
	if body.Text != decodedTurkish {
		t.Errorf("Text = %q, want %q", body.Text, decodedTurkish)
	}
}

// Real mail is multipart. A repair that only handled a single-part message
// would fix almost nothing, because almost nothing is single-part.
func TestForcingACharsetWalksIntoMultipart(t *testing.T) {
	var body bytes.Buffer
	body.WriteString("--sinir\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n")
	body.Write(latin5Turkish)
	body.WriteString("\r\n--sinir\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>")
	body.Write(latin5Turkish)
	body.WriteString("</p>\r\n--sinir--\r\n")

	raw := rawMessage([]string{
		"Subject: Rapor",
		"MIME-Version: 1.0",
		`Content-Type: multipart/alternative; boundary="sinir"`,
	}, body.Bytes())

	got, err := DecodeBodyForcingCharset(raw, "iso-8859-9")
	if err != nil {
		t.Fatalf("DecodeBodyForcingCharset() error: %v", err)
	}
	if got.Text != decodedTurkish {
		t.Errorf("Text = %q, want %q", got.Text, decodedTurkish)
	}
	if !strings.Contains(got.HTML, decodedTurkish) {
		t.Errorf("HTML = %q, want it to contain %q", got.HTML, decodedTurkish)
	}
}

// The common repair is in the other direction: a message that really is UTF-8
// but declares something else, so the client mangled it on the way in.
func TestForcingUTF8Works(t *testing.T) {
	raw := rawMessage([]string{
		"Content-Type: text/plain; charset=iso-8859-1",
	}, []byte(decodedTurkish))

	body, err := DecodeBodyForcingCharset(raw, "utf-8")
	if err != nil {
		t.Fatalf("DecodeBodyForcingCharset() error: %v", err)
	}
	if body.Text != decodedTurkish {
		t.Errorf("Text = %q, want %q", body.Text, decodedTurkish)
	}
}

// Every name the picker offers has to actually resolve, or the menu is
// advertising repairs that cannot be made.
func TestEveryOfferedCharsetResolves(t *testing.T) {
	for _, name := range RepairCharsets {
		raw := rawMessage([]string{"Content-Type: text/plain"}, []byte("merhaba"))
		if _, err := DecodeBodyForcingCharset(raw, name); err != nil {
			t.Errorf("the picker offers %q but it does not work: %v", name, err)
		}
	}
}

func TestForcingAnUnknownCharsetIsRefused(t *testing.T) {
	raw := rawMessage([]string{"Content-Type: text/plain"}, []byte("merhaba"))

	if _, err := DecodeBodyForcingCharset(raw, "definitely-not-a-charset"); err == nil {
		t.Error("DecodeBodyForcingCharset() accepted a charset that does not exist")
	}
	if _, err := DecodeBodyForcingCharset(raw, ""); err == nil {
		t.Error("DecodeBodyForcingCharset() accepted an empty charset name")
	}
}

// A message with nothing but an attachment has no text to repair, and saying
// so is better than handing back an empty body that looks like a lost message.
func TestRepairingAMessageWithNoTextPartIsAnError(t *testing.T) {
	var body bytes.Buffer
	body.WriteString("--sinir\r\nContent-Type: application/pdf\r\n\r\n%PDF-1.4\r\n--sinir--\r\n")

	raw := rawMessage([]string{
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="sinir"`,
	}, body.Bytes())

	if _, err := DecodeBodyForcingCharset(raw, "utf-8"); err == nil {
		t.Error("a message with no text part was repaired into something")
	}
}

// Nesting is attacker-controlled. An uncapped recursive walk over a message
// that wraps multipart in multipart a thousand times is a stack overflow
// triggered by opening mail.
func TestDeeplyNestedMultipartDoesNotRunAway(t *testing.T) {
	const depth = 200

	body := []byte("son")
	for i := 0; i < depth; i++ {
		boundary := "s" + strings.Repeat("x", i%5) + itoaSmall(i)
		var next bytes.Buffer
		next.WriteString("--" + boundary + "\r\n")
		next.WriteString("Content-Type: multipart/mixed; boundary=" + quoteBoundary(i) + "\r\n\r\n")
		next.Write(body)
		next.WriteString("\r\n--" + boundary + "--\r\n")
		body = next.Bytes()
	}

	// The assertion is that this returns at all rather than exhausting the
	// stack; whether it finds the innermost text is beside the point.
	_, _ = DecodeBodyForcingCharset(rawMessage([]string{
		"MIME-Version: 1.0",
		`Content-Type: multipart/mixed; boundary="s0"`,
	}, body), "utf-8")
}

func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func quoteBoundary(i int) string {
	return `"s` + itoaSmall(i+1) + `"`
}
