package imapx

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/emersion/go-message"
	// Registers the non-UTF-8 charset decoders. Without this import only
	// UTF-8 and US-ASCII are recognised, and the ISO-8859-9 mail Turkish
	// corporate systems still send would fall through to the raw fallback.
	_ "github.com/emersion/go-message/charset"
	"github.com/emersion/go-message/mail"
)

// splitBodyParts walks a raw RFC 5322 message and returns its HTML and plain
// text representations.
//
// Character set decoding is handled by go-message, which matters for the
// ISO-8859-9 mail Turkish corporate systems still send: without it, subjects
// and bodies arrive as mojibake.
func splitBodyParts(raw []byte) (html, text string, err error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		// A message we cannot parse as MIME is not necessarily worthless: fall
		// back to showing its body as plain text rather than nothing at all.
		if message.IsUnknownCharset(err) {
			return "", bodyAfterHeaders(raw), nil
		}
		return "", "", err
	}
	defer func() { _ = mr.Close() }()

	for {
		part, partErr := mr.NextPart()
		if errors.Is(partErr, io.EOF) {
			break
		}
		if partErr != nil {
			// An unknown charset in one part is not fatal: keep whatever the
			// other parts decoded to rather than showing the user nothing.
			if message.IsUnknownCharset(partErr) {
				continue
			}
			return "", "", partErr
		}

		inline, ok := part.Header.(*mail.InlineHeader)
		if !ok {
			// An attachment part; metadata comes from BODYSTRUCTURE instead.
			continue
		}

		contentType, _, ctErr := inline.ContentType()
		if ctErr != nil {
			continue
		}
		body, readErr := io.ReadAll(part.Body)
		if readErr != nil {
			continue
		}

		// Keep the first of each kind: a multipart/alternative lists them in
		// increasing order of richness, but later parts are usually the same
		// content repeated.
		switch {
		case strings.EqualFold(contentType, "text/html") && html == "":
			html = string(body)
		case strings.EqualFold(contentType, "text/plain") && text == "":
			text = string(body)
		}
	}
	return html, text, nil
}

// bodyAfterHeaders returns everything past the header/body separator, so a
// message we could not parse still shows its content rather than a wall of
// raw IMAP headers.
func bodyAfterHeaders(raw []byte) string {
	for _, sep := range []string{"\r\n\r\n", "\n\n"} {
		if i := strings.Index(string(raw), sep); i >= 0 {
			return string(raw[i+len(sep):])
		}
	}
	return string(raw)
}
