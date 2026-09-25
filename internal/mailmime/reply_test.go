package mailmime

import (
	"strings"
	"testing"
	"time"
)

// Every prefix a client invents is another one other clients fail to
// recognise, and a thread that crosses two languages grows "Re: AW: Re: AW:"
// down the subject line.
func TestReplySubjectWritesOnePrefixAndStripsTheRest(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Toplantı", "Re: Toplantı"},
		// Already answered once: one prefix, not two.
		{"Re: Toplantı", "Re: Toplantı"},
		{"RE: Toplantı", "Re: Toplantı"},
		{"re:Toplantı", "Re: Toplantı"},
		// A thread that crossed languages.
		{"AW: Re: Toplantı", "Re: Toplantı"},
		{"Yanıt: Toplantı", "Re: Toplantı"},
		{"Re: Re: Re: Toplantı", "Re: Toplantı"},
		// Whitespace around the original.
		{"   Toplantı  ", "Re: Toplantı"},
		// A subject that only looks like a prefix is still a subject.
		{"Rethinking the plan", "Re: Rethinking the plan"},
		{"", "Re: "},
	}

	for _, c := range cases {
		if got := ReplySubject(c.in); got != c.want {
			t.Errorf("ReplySubject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestForwardSubjectStripsBothKinds(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Toplantı", "Fwd: Toplantı"},
		{"Fwd: Toplantı", "Fwd: Toplantı"},
		{"FW: Toplantı", "Fwd: Toplantı"},
		// Forwarding a reply drops the Re: too: the thing being forwarded is a
		// message, and how it came to exist is not in its subject.
		{"Re: Toplantı", "Fwd: Toplantı"},
	}
	for _, c := range cases {
		if got := ForwardSubject(c.in); got != c.want {
			t.Errorf("ForwardSubject(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestQuoteMarksEveryLine(t *testing.T) {
	when := time.Date(2026, 9, 25, 14, 30, 0, 0, time.UTC)

	got := Quote("Yazan <u@example.com>", when, "birinci\nikinci")

	if !strings.HasPrefix(got, "On 25 September 2026 at 14:30, Yazan <u@example.com> wrote:") {
		t.Errorf("the attribution line is wrong:\n%s", got)
	}
	if !strings.Contains(got, "> birinci\n> ikinci\n") {
		t.Errorf("the body was not marked:\n%s", got)
	}
}

// A blank line without a marker reads to the next client as the end of the
// quote, which is how a quoted paragraph turns into half a quote and half new
// text attributed to the wrong person.
func TestQuoteMarksBlankLinesToo(t *testing.T) {
	got := Quote("Yazan", time.Now(), "birinci\n\nucuncu")

	if !strings.Contains(got, "\n>\n") {
		t.Errorf("a blank line was left unmarked:\n%s", got)
	}
	// And the marker on a blank line has no trailing space, which would be
	// stripped in transit anyway.
	if strings.Contains(got, "> \n") {
		t.Errorf("a blank quote line carries a trailing space:\n%s", got)
	}
}

// A reply quoted three deep should read as three levels everywhere.
//
// Each level adds its own "> ", so the second reads "> > " rather than ">>".
// That is what every plain-text client produces, and it is what makes the
// depth countable: the markers stay separable, so a client that re-flows the
// text can still tell two levels from one.
func TestQuotingAQuoteDeepensIt(t *testing.T) {
	once := Quote("A", time.Now(), "metin")
	twice := Quote("B", time.Now(), once)

	if !strings.Contains(twice, "> > metin") {
		t.Errorf("the second level did not deepen the first:\n%s", twice)
	}
	// The inner attribution is quoted too, so the reader can see who said what
	// at each level.
	if !strings.Contains(twice, "> On ") {
		t.Errorf("the inner attribution was lost:\n%s", twice)
	}
}

func TestQuoteHandlesEveryLineEnding(t *testing.T) {
	for _, body := range []string{"bir\nkin", "bir\r\nkin", "bir\rkin"} {
		got := Quote("A", time.Now(), body)
		if strings.Contains(got, "\r") {
			t.Errorf("a carriage return survived quoting %q:\n%q", body, got)
		}
		if !strings.Contains(got, "> bir\n> kin") {
			t.Errorf("quoting %q produced:\n%q", body, got)
		}
	}
}

func TestQuoteWithoutADateStillAttributes(t *testing.T) {
	got := Quote("Yazan", time.Time{}, "metin")

	if !strings.HasPrefix(got, "Yazan wrote:") {
		t.Errorf("the attribution is wrong with no date:\n%s", got)
	}
}

// Threading is what makes a reply appear under what it answers.
func TestReferencesAreTheParentsPlusItsOwnID(t *testing.T) {
	got := ReplyReferences([]string{"root@example.com"}, "parent@example.com")

	if len(got) != 2 || got[0] != "root@example.com" || got[1] != "parent@example.com" {
		t.Errorf("References = %v", got)
	}
}

func TestReferencesStartFromNothingOnAFirstReply(t *testing.T) {
	got := ReplyReferences(nil, "first@example.com")

	if len(got) != 1 || got[0] != "first@example.com" {
		t.Errorf("References = %v", got)
	}
}

// A header that repeats itself is one more thing for a threading algorithm to
// trip over, and a client that already appended its own id leaves it twice.
func TestReferencesAreDeduplicated(t *testing.T) {
	got := ReplyReferences(
		[]string{"root@example.com", "parent@example.com"}, "parent@example.com")

	if len(got) != 2 {
		t.Errorf("References = %v, want the duplicate dropped", got)
	}
}

func TestAnEmptyParentIDIsNotAppended(t *testing.T) {
	got := ReplyReferences([]string{"root@example.com"}, "")

	if len(got) != 1 || got[0] != "root@example.com" {
		t.Errorf("References = %v", got)
	}
}

// A long thread otherwise grows a header some servers refuse. The root is what
// identifies the thread and the recent ones identify the position in it, so
// the middle is what goes.
func TestALongThreadKeepsTheRootAndTheRecentOnes(t *testing.T) {
	var parents []string
	for i := range 40 {
		parents = append(parents, string(rune('a'+i%26))+"-"+itoa(i)+"@example.com")
	}

	got := ReplyReferences(parents, "newest@example.com")

	if len(got) != 20 {
		t.Fatalf("References has %d entries, want it capped at 20", len(got))
	}
	if got[0] != parents[0] {
		t.Errorf("the root was dropped: first entry is %q", got[0])
	}
	if got[len(got)-1] != "newest@example.com" {
		t.Errorf("the parent was dropped: last entry is %q", got[len(got)-1])
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
