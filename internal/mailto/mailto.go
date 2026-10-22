// Package mailto parses a mailto: URL into a message the composer can open.
//
// RFC 6068, and deliberately only the part a desktop client needs: recipients,
// subject, body and the copy lines. A URL is an untrusted string from another
// application, so every field is length-bounded and headers that could inject
// a line are refused rather than cleaned up silently.
package mailto

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"
)

// Limits keep one external link from turning into an unbounded draft or a
// header the MIME builder would have to repair.
const (
	maxURLLength   = 8192
	maxBodyBytes   = 64 << 10
	maxSubjectRune = 500
	maxRecipients  = 50
)

// Draft is what the composer opens with.
type Draft struct {
	To      string
	Cc      string
	Bcc     string
	Subject string
	Body    string
	// InReplyTo is the Message-ID a mailto can carry, without angle brackets.
	InReplyTo string
}

// Parse reads a mailto: URL. The scheme is required; a bare address is not a
// URL this application registered for, and guessing would make a typo into a
// message.
func Parse(raw string) (Draft, error) {
	if len(raw) > maxURLLength {
		return Draft{}, fmt.Errorf("mailto: the link is longer than %d bytes", maxURLLength)
	}
	if !strings.HasPrefix(strings.ToLower(raw), "mailto:") {
		return Draft{}, fmt.Errorf("mailto: %q does not start with mailto:", raw)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Draft{}, fmt.Errorf("mailto: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "mailto") {
		return Draft{}, fmt.Errorf("mailto: %q is not a mailto link", raw)
	}

	// net/url puts "a@example.com?subject=x" in Opaque, and the empty form
	// "mailto:?to=a@example.com" in RawQuery only. Both are legal. Opaque is
	// not unescaped by url.Parse, unlike Path.
	to := u.Path
	if u.Opaque != "" {
		decoded, err := url.PathUnescape(u.Opaque)
		if err != nil {
			return Draft{}, fmt.Errorf("mailto: malformed address: %w", err)
		}
		to = decoded
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return Draft{}, fmt.Errorf("mailto: malformed query: %w", err)
	}
	if to == "" {
		to = query.Get("to")
	}

	d := Draft{
		To:      to,
		Cc:      query.Get("cc"),
		Bcc:     query.Get("bcc"),
		Subject: query.Get("subject"),
		Body:    query.Get("body"),
		InReplyTo: strings.Trim(
			strings.TrimSpace(query.Get("in-reply-to")), "<>"),
	}

	if len(d.Body) > maxBodyBytes {
		return Draft{}, fmt.Errorf("mailto: the body is longer than %d bytes", maxBodyBytes)
	}
	if len([]rune(d.Subject)) > maxSubjectRune {
		return Draft{}, fmt.Errorf("mailto: the subject is longer than %d characters", maxSubjectRune)
	}

	// Header fields cannot carry a line break. A body can: a paragraph is a
	// legitimate line break, and the MIME writer normalises it later.
	for field, value := range map[string]string{
		"to": d.To, "cc": d.Cc, "bcc": d.Bcc, "subject": d.Subject,
		"in-reply-to": d.InReplyTo,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return Draft{}, fmt.Errorf("mailto: %s contains a line break", field)
		}
	}

	for field, value := range map[string]string{"to": d.To, "cc": d.Cc, "bcc": d.Bcc} {
		if value == "" {
			continue
		}
		if err := checkAddressList(value); err != nil {
			return Draft{}, fmt.Errorf("mailto: %s: %w", field, err)
		}
		if countAddresses(value) > maxRecipients {
			return Draft{}, fmt.Errorf("mailto: more than %d recipients in %s", maxRecipients, field)
		}
	}

	return d, nil
}

func checkAddressList(value string) error {
	parsed, err := mail.ParseAddressList(value)
	if err != nil {
		return fmt.Errorf("not an address list: %w", err)
	}
	if len(parsed) == 0 {
		return fmt.Errorf("no addresses")
	}
	return nil
}

func countAddresses(value string) int {
	// The list has already parsed; counting commas outside quotes is enough
	// for the cap, and a wrong count here only tightens or loosens a limit.
	quoted := false
	n := 1
	for _, r := range value {
		switch r {
		case '"':
			quoted = !quoted
		case ',':
			if !quoted {
				n++
			}
		}
	}
	return n
}
