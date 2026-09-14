package imapx

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/ianaindex"
)

// RepairCharsets are the encodings a reader can force a message into.
//
// A short list on purpose. The full IANA registry is hundreds of names, almost
// all of which have never been wrong on a message anyone here will receive;
// offering them would turn a repair into a search. These are the ones that
// actually show up mislabelled: Latin-5 and its Windows superset because
// Turkish corporate systems still send them, the Latin-1/Windows-1252 pair
// because half the world's legacy mail claims one and is the other, and the
// rest because they are the common single-byte families.
//
// UTF-8 is first because the most common repair is in the other direction: a
// message that really is UTF-8 but declares something else.
var RepairCharsets = []string{
	"utf-8",
	"iso-8859-9",
	"windows-1254",
	"iso-8859-1",
	"windows-1252",
	"iso-8859-2",
	"windows-1250",
	"windows-1251",
}

// maxMIMEDepth caps how far into a nested message the walk will go.
//
// Nesting is attacker-controlled: a message can wrap multipart in multipart as
// many times as the sender likes, and an uncapped recursive walk over one is a
// stack overflow triggered by opening mail.
const maxMIMEDepth = 10

// DecodeBodyForcingCharset re-reads a raw message, decoding every text part
// with the named character set instead of the one the message declares.
//
// This exists because the declared charset is frequently a lie. A message
// whose headers say UTF-8 but whose bytes are Latin-5 renders as mojibake, and
// no amount of correctness in the normal path fixes it — the normal path is
// doing exactly what it was told. The reader can see what it should have said,
// so the reader gets to say it.
//
// The MIME walk here is deliberately not go-message's: that library applies
// the declared charset while it parses, which is the thing being overridden.
// Doing the transfer decoding and the charset decoding as two separate steps
// is what makes the second one substitutable.
func DecodeBodyForcingCharset(raw []byte, charsetName string) (Body, error) {
	enc, err := lookupCharset(charsetName)
	if err != nil {
		return Body{}, err
	}

	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return Body{}, fmt.Errorf("imapx: reading the message to repair it: %w", err)
	}

	var out Body
	walkForCharset(textproto.MIMEHeader(msg.Header), msg.Body, enc, 0, &out)
	if out.HTML == "" && out.Text == "" {
		return Body{}, fmt.Errorf("imapx: the message has no text part to repair")
	}
	return out, nil
}

// walkForCharset descends the MIME tree, filling in the first HTML and the
// first plain-text part it finds.
//
// Errors are swallowed rather than returned. A repair is already a recovery
// from a malformed message; refusing to show the parts that did decode because
// a sibling part did not would leave the reader exactly where they started.
func walkForCharset(header textproto.MIMEHeader, body io.Reader,
	enc encoding.Encoding, depth int, out *Body) {

	if depth > maxMIMEDepth {
		return
	}

	mediaType, params := contentTypeOf(header)

	if strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			// NextRawPart, not NextPart: NextPart silently undoes
			// quoted-printable for us, so half the parts would arrive decoded
			// and half not, and decodePart below would then mangle the ones
			// that were already done.
			part, err := mr.NextRawPart()
			if err != nil {
				return
			}
			walkForCharset(part.Header, part, enc, depth+1, out)
			if out.HTML != "" && out.Text != "" {
				return
			}
		}
	}

	isHTML := strings.EqualFold(mediaType, "text/html")
	isText := strings.EqualFold(mediaType, "text/plain")
	if !isHTML && !isText {
		return
	}
	if (isHTML && out.HTML != "") || (isText && out.Text != "") {
		return
	}

	rawPart, err := io.ReadAll(body)
	if err != nil {
		return
	}
	decoded, err := decodePart(rawPart, header.Get("Content-Transfer-Encoding"))
	if err != nil {
		return
	}

	text, err := enc.NewDecoder().Bytes(decoded)
	if err != nil {
		// A byte sequence the chosen encoding cannot represent means the
		// reader guessed wrong. Showing them the undecoded bytes is more
		// useful than an error, because seeing it still look wrong is how they
		// know to try the next one.
		text = decoded
	}

	if isHTML {
		out.HTML = string(text)
	} else {
		out.Text = string(text)
	}
}

// contentTypeOf reads a part's media type, defaulting the way RFC 2045 says to
// when the header is missing or unparseable.
func contentTypeOf(header textproto.MIMEHeader) (string, map[string]string) {
	value := header.Get("Content-Type")
	if value == "" {
		return "text/plain", nil
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil {
		return "text/plain", nil
	}
	return strings.ToLower(mediaType), params
}

// lookupCharset resolves an encoding name.
//
// The IANA table is tried first because it is the registry mail headers name,
// and the WHATWG one second because it knows the aliases browsers accept —
// including the ones mail clients copied from browsers.
func lookupCharset(name string) (encoding.Encoding, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("imapx: no character set named")
	}

	if enc, err := ianaindex.MIME.Encoding(name); err == nil && enc != nil {
		return enc, nil
	}
	if enc, err := htmlindex.Get(name); err == nil && enc != nil {
		return enc, nil
	}
	return nil, fmt.Errorf("imapx: unknown character set %q", name)
}
