package logging

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

// Redacted replaces a value that must never reach the log.
const Redacted = "[redacted]"

// Attributes with these keys carry mail content or credentials and are
// replaced wholesale rather than scrubbed: there is no version of a subject
// line that is safe to write down.
//
// Two lists, because one matching rule cannot serve both. Distinctive words
// are matched as substrings, so "message_subject" and "refresh_token" are
// covered without enumerating every variation someone might invent. Short
// words cannot be: "cc" is a substring of "account", and "to" of
// "total_count", so matching those loosely would redact the very fields that
// make a log readable.
var forbiddenSubstrings = []string{
	// Mail content
	"subject", "snippet", "preview", "filename", "attachment",
	// Identities
	"email", "address", "addr", "sender", "recipient",
	// Credentials
	"password", "passwd", "secret", "token", "credential", "authorization",
	"cookie", "session", "apikey", "api_key",
}

// forbiddenSegments are matched against whole underscore- or dash-separated
// segments of a key, so "from_name" and "cc_addrs" are caught while
// "account_id" and "total_count" are not.
var forbiddenSegments = map[string]bool{
	"body": true, "html": true, "text": true,
	"from": true, "to": true, "cc": true, "bcc": true,
	"auth": true, "user": true, "username": true,
}

var (
	// emailAddress catches an address embedded in free text — most often
	// inside a wrapped error message rather than as its own attribute.
	emailAddress = regexp.MustCompile(`[\w.+-]+@[\w-]+(?:\.[\w-]+)+`)

	// bearerToken catches the XOAUTH2 and HTTP forms.
	bearerToken = regexp.MustCompile(`(?i)(bearer\s+)\S+`)
)

// RedactingHandler strips mail content and credentials before they are
// written.
//
// It exists because of the export button. A privacy-focused client that
// invites users to attach their log to a bug report has to guarantee the log
// is safe to attach; asking developers to remember is not a guarantee.
//
// What it cannot do: recognise an arbitrary secret sitting in free text. The
// key-based rules cover the shapes we know, and the standing rule — never log
// content — covers the rest.
type RedactingHandler struct {
	inner slog.Handler
}

func NewRedactingHandler(inner slog.Handler) *RedactingHandler {
	return &RedactingHandler{inner: inner}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, scrub(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, clean)
}

func (h *RedactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		redacted = append(redacted, redactAttr(a))
	}
	return &RedactingHandler{inner: h.inner.WithAttrs(redacted)}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	if isForbiddenKey(a.Key) {
		return slog.String(a.Key, Redacted)
	}

	// A group's members need the same treatment as top-level attributes.
	if a.Value.Kind() == slog.KindGroup {
		members := a.Value.Group()
		out := make([]any, 0, len(members))
		for _, member := range members {
			out = append(out, redactAttr(member))
		}
		return slog.Group(a.Key, out...)
	}

	// Resolve LogValuer and error values to their string form so the patterns
	// below can see inside them. A wrapped error is the most common way a
	// token reaches a log by accident.
	resolved := a.Value.Resolve()
	if resolved.Kind() == slog.KindAny {
		if err, ok := resolved.Any().(error); ok {
			return slog.String(a.Key, scrub(err.Error()))
		}
	}
	if resolved.Kind() == slog.KindString {
		return slog.String(a.Key, scrub(resolved.String()))
	}
	return slog.Attr{Key: a.Key, Value: resolved}
}

func isForbiddenKey(key string) bool {
	lower := strings.ToLower(key)

	for _, forbidden := range forbiddenSubstrings {
		if strings.Contains(lower, forbidden) {
			return true
		}
	}
	for _, segment := range strings.FieldsFunc(lower, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == ' '
	}) {
		if forbiddenSegments[segment] {
			return true
		}
	}
	return false
}

// scrub removes the sensitive shapes we can recognise inside free text.
func scrub(s string) string {
	s = emailAddress.ReplaceAllString(s, "[redacted-address]")
	s = bearerToken.ReplaceAllString(s, "${1}[redacted-token]")
	return s
}
