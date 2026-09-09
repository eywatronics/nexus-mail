package imapx

import (
	"strings"
	"testing"
	"time"
)

func TestThreadKeyPrefersReferencesRoot(t *testing.T) {
	// A reply carries the whole ancestry; the first entry is the thread root,
	// which is what keeps every message in a long chain together.
	got := ThreadKey("<c@x>", "<b@x>", []string{"<a@x>", "<b@x>"}, "Re: Invoice")
	if got != "<a@x>" {
		t.Errorf("ThreadKey() = %q, want the References root <a@x>", got)
	}
}

func TestThreadKeyFallsBackToInReplyTo(t *testing.T) {
	// Some clients send In-Reply-To without References.
	got := ThreadKey("<c@x>", "<b@x>", nil, "Re: Invoice")
	if got != "<b@x>" {
		t.Errorf("ThreadKey() = %q, want <b@x>", got)
	}
}

func TestThreadKeyTakesTheFirstInReplyToID(t *testing.T) {
	// In-Reply-To may list several ids; the first is the direct parent, and
	// the whole string would never match another message's key.
	got := ThreadKey("<c@x>", "<b@x> <a@x>", nil, "Re: Invoice")
	if got != "<b@x>" {
		t.Errorf("ThreadKey() = %q, want the first id <b@x>", got)
	}
}

func TestThreadKeyUsesOwnMessageIDForThreadStart(t *testing.T) {
	got := ThreadKey("<a@x>", "", nil, "Invoice")
	if got != "<a@x>" {
		t.Errorf("ThreadKey() = %q, want its own Message-ID <a@x>", got)
	}
}

func TestThreadKeyFallsBackToNormalisedSubject(t *testing.T) {
	// Mailing lists and some corporate gateways strip Message-ID entirely.
	// The subject is the last resort, normalised so replies land in the same
	// thread as the message they answer.
	want := ThreadKey("", "", nil, "Invoice 2026")

	cases := []string{
		"Re: Invoice 2026",
		"RE: Invoice 2026",
		"Fwd: Invoice 2026",
		"FW: Invoice 2026",
		"Re: Re: Invoice 2026",
		"Re: Fwd: Re: Invoice 2026",
		"  Invoice 2026  ",
		"Invoice  2026",
	}
	for _, subject := range cases {
		if got := ThreadKey("", "", nil, subject); got != want {
			t.Errorf("ThreadKey(subject=%q) = %q, want %q", subject, got, want)
		}
	}
}

// Turkish Outlook prefixes replies with YNT (yanıt) and forwards with İLT
// (ilet). Without these a single corporate Turkish conversation splits into
// two threads — and Turkish is this project's primary user language.
func TestThreadKeyNormalisesTurkishReplyPrefixes(t *testing.T) {
	want := ThreadKey("", "", nil, "Şubat faturası")

	for _, subject := range []string{
		"YNT: Şubat faturası",
		"Ynt: Şubat faturası",
		"İLT: Şubat faturası",
		"YNT: YNT: Şubat faturası",
	} {
		if got := ThreadKey("", "", nil, subject); got != want {
			t.Errorf("ThreadKey(subject=%q) = %q, want %q", subject, got, want)
		}
	}
}

func TestThreadKeyDistinguishesDifferentSubjects(t *testing.T) {
	a := ThreadKey("", "", nil, "Invoice 2026")
	b := ThreadKey("", "", nil, "Invoice 2027")
	if a == b {
		t.Error("different subjects produced the same thread key")
	}
}

func TestReconcileDatePrefersInternalDateWhenTheHeaderIsUnusable(t *testing.T) {
	internal := time.Unix(1700000000, 0)

	cases := []struct {
		name   string
		header time.Time
		want   time.Time
	}{
		{"unparseable header leaves a zero time", time.Time{}, internal},
		{"absurd future date", internal.Add(365 * 24 * time.Hour), internal},
		{"just past the skew limit", internal.Add(49 * time.Hour), internal},
		// Real mail drifts by hours through timezone mistakes; that is normal
		// and must not be overridden.
		{"plausible timezone drift is kept", internal.Add(6 * time.Hour), internal.Add(6 * time.Hour)},
		{"a date in the past is kept", internal.Add(-72 * time.Hour), internal.Add(-72 * time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconcileDate(tc.header, internal); !got.Equal(tc.want) {
				t.Errorf("reconcileDate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReconcileDateKeepsTheHeaderWhenThereIsNoInternalDate(t *testing.T) {
	header := time.Unix(1700000000, 0)
	if got := reconcileDate(header, time.Time{}); !got.Equal(header) {
		t.Errorf("reconcileDate() = %v, want the header date %v", got, header)
	}
}

func TestSplitBodyPartsReadsBothRepresentations(t *testing.T) {
	raw := "From: a@example.com\r\n" +
		"To: b@example.com\r\n" +
		"Subject: Both\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=BOUND\r\n" +
		"\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"plain version\r\n" +
		"--BOUND\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"\r\n" +
		"<p>html version</p>\r\n" +
		"--BOUND--\r\n"

	html, text, err := splitBodyParts([]byte(raw))
	if err != nil {
		t.Fatalf("splitBodyParts() error: %v", err)
	}
	if !strings.Contains(html, "html version") {
		t.Errorf("html = %q, want it to contain the HTML part", html)
	}
	if !strings.Contains(text, "plain version") {
		t.Errorf("text = %q, want it to contain the plain part", text)
	}
}

// Turkish corporate systems still send ISO-8859-9. Without charset decoding
// these arrive as mojibake, which looks like data loss to the user.
func TestSplitBodyPartsDecodesISO88599(t *testing.T) {
	// "Şubat faturası" in ISO-8859-9: Ş is 0xDD, ı is 0xFD.
	body := []byte("\xdcr\xfcn: \xdeubat faturas\xfd")

	raw := append([]byte(
		"From: a@example.com\r\n"+
			"To: b@example.com\r\n"+
			"Subject: Fatura\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/plain; charset=iso-8859-9\r\n"+
			"\r\n"), body...)

	_, text, err := splitBodyParts(raw)
	if err != nil {
		t.Fatalf("splitBodyParts() error: %v", err)
	}
	if !strings.Contains(text, "Şubat") {
		t.Errorf("text = %q, want the ISO-8859-9 bytes decoded to Şubat", text)
	}
	if !strings.Contains(text, "faturası") {
		t.Errorf("text = %q, want faturası", text)
	}
}

// An unknown charset must not lose the message. Showing something imperfect
// beats showing an empty reading pane.
func TestSplitBodyPartsSurvivesAnUnknownCharset(t *testing.T) {
	raw := "From: a@example.com\r\n" +
		"To: b@example.com\r\n" +
		"Subject: Odd\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=x-not-a-real-charset\r\n" +
		"\r\n" +
		"readable anyway\r\n"

	html, text, err := splitBodyParts([]byte(raw))
	if err != nil {
		t.Fatalf("splitBodyParts() returned an error for an unknown charset: %v", err)
	}
	if html == "" && text == "" {
		t.Error("both representations are empty; an unknown charset lost the whole message")
	}
}

func TestSplitBodyPartsOnAPlainTextOnlyMessage(t *testing.T) {
	raw := "From: a@example.com\r\n" +
		"To: b@example.com\r\n" +
		"Subject: Plain\r\n" +
		"\r\n" +
		"just text\r\n"

	html, text, err := splitBodyParts([]byte(raw))
	if err != nil {
		t.Fatalf("splitBodyParts() error: %v", err)
	}
	if !strings.Contains(text, "just text") {
		t.Errorf("text = %q, want it to contain the body", text)
	}
	// No HTML part exists, so the reader falls back to the text one.
	if html != "" {
		t.Errorf("html = %q, want empty for a text-only message", html)
	}
}

func TestNormaliseSubjectCollapsesWhitespaceAndCase(t *testing.T) {
	// Subjects wrap across lines in transit, so the normalised form has to
	// ignore how the whitespace landed.
	if got, want := normaliseSubject("  Quarterly\t Report  "), "quarterly report"; got != want {
		t.Errorf("normaliseSubject() = %q, want %q", got, want)
	}
}
