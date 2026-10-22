package mailto

import (
	"strings"
	"testing"
)

func TestParseReadsTheComposerFields(t *testing.T) {
	got, err := Parse(
		"mailto:ali@example.com?cc=destek%40example.com&bcc=gizli%40example.com" +
			"&subject=Merhaba%20d%C3%BCnya&body=Birinci%20sat%C4%B1r%0A%C4%B0kinci",
	)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got.To != "ali@example.com" {
		t.Errorf("To = %q", got.To)
	}
	if got.Cc != "destek@example.com" || got.Bcc != "gizli@example.com" {
		t.Errorf("Cc/Bcc = %q / %q", got.Cc, got.Bcc)
	}
	if got.Subject != "Merhaba dünya" {
		t.Errorf("Subject = %q", got.Subject)
	}
	if got.Body != "Birinci satır\nİkinci" {
		t.Errorf("Body = %q", got.Body)
	}
}

// RFC 6068 permits both mailto:a@b and mailto:?to=a@b.
func TestParseAcceptsRecipientsFromThePathOrTheQuery(t *testing.T) {
	for _, raw := range []string{
		"mailto:?to=ali@example.com&subject=Konu",
		"mailto:ali@example.com?subject=Konu",
	} {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q) error: %v", raw, err)
		}
		if got.To != "ali@example.com" {
			t.Errorf("Parse(%q).To = %q", raw, got.To)
		}
	}
}

func TestParseKeepsADisplayNameWithAComma(t *testing.T) {
	got, err := Parse(
		"mailto:%22Kabatepe%2C%20%C4%B0smet%22%20%3Ca%40example.com%3E",
	)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got.To != `"Kabatepe, İsmet" <a@example.com>` {
		t.Errorf("To = %q", got.To)
	}
}

// A mailto is a string another application supplies. A newline in a header is
// how one message becomes two, so it is a refusal rather than a sanitisation.
func TestParseRefusesHeaderInjection(t *testing.T) {
	for _, raw := range []string{
		"mailto:a@example.com?subject=hi%0ABcc:%20evil@example.com",
		"mailto:?to=b@example.com%0D%0AX-Evil:%20yes",
		"mailto:a@example.com?cc=b@example.com%0ASubject:%20no",
	} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) accepted a line break", raw)
		}
	}
}

func TestParseRefusesWhatItCannotSend(t *testing.T) {
	for _, raw := range []string{
		"",
		"a@example.com",
		"https://example.com?to=a@example.com",
		"mailto:not an address",
		"mailto:?to=also not an address",
	} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) accepted an unusable link", raw)
		}
	}
}

func TestParseRejectsAnOversizedLinkOrBody(t *testing.T) {
	tooLong := "mailto:a@example.com?body=" + strings.Repeat("x", maxURLLength)
	if _, err := Parse(tooLong); err == nil {
		t.Error("Parse() accepted an oversized URL")
	}

	tooBigBody := "mailto:a@example.com?body=" + strings.Repeat("x", maxBodyBytes+1)
	if _, err := Parse(tooBigBody); err == nil {
		t.Error("Parse() accepted an oversized body")
	}
}

// A body may have newlines; a subject may not, and the difference is what the
// injection test above is about.
func TestParseAllowsNewlinesInTheBody(t *testing.T) {
	got, err := Parse("mailto:a@example.com?body=one%0Atwo%0A%0Athree")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got.Body != "one\ntwo\n\nthree" {
		t.Errorf("Body = %q", got.Body)
	}
}

func TestParseReadsInReplyTo(t *testing.T) {
	got, err := Parse("mailto:a@example.com?in-reply-to=%3Cparent@example.com%3E")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if got.InReplyTo != "parent@example.com" {
		t.Errorf("InReplyTo = %q", got.InReplyTo)
	}
}
