package mailhtml

import (
	"strings"
	"testing"
)

func mustHighlight(t *testing.T, body, query string, current int) HighlightResult {
	t.Helper()

	res, err := Highlight(body, query, current)
	if err != nil {
		t.Fatalf("Highlight(%q, %q) error: %v", body, query, err)
	}
	return res
}

func TestEveryOccurrenceIsWrapped(t *testing.T) {
	res := mustHighlight(t, "<p>fatura, fatura ve yine fatura</p>", "fatura", 0)

	if res.Count != 3 {
		t.Errorf("Count = %d, want 3", res.Count)
	}
	if got := strings.Count(res.HTML, "<mark"); got != 3 {
		t.Errorf("the body holds %d marks, want 3: %s", got, res.HTML)
	}
	// The text itself must survive untouched — a find that rewrites the message
	// is worse than no find.
	if !strings.Contains(res.HTML, "fatura, fatura ve yine fatura") {
		// Only true once the marks are removed, which is what the next check is.
		stripped := strings.NewReplacer(
			`<mark class="nx-find" id="nx-find-current">`, "",
			`<mark class="nx-find">`, "", "</mark>", "").Replace(res.HTML)
		if stripped != "<p>fatura, fatura ve yine fatura</p>" {
			t.Errorf("the text changed: %s", stripped)
		}
	}
}

// Only one match is the current one, because only one place can be scrolled to.
func TestOnlyTheCurrentMatchCarriesTheScrollTarget(t *testing.T) {
	res := mustHighlight(t, "<p>bir iki bir iki bir</p>", "bir", 1)

	if res.Count != 3 {
		t.Fatalf("Count = %d, want 3", res.Count)
	}
	if got := strings.Count(res.HTML, currentMarkID); got != 1 {
		t.Errorf("%d elements carry the scroll target, want 1: %s", got, res.HTML)
	}

	// And it is the second one: the first "bir" is plain, the second is current.
	first := strings.Index(res.HTML, "<mark")
	target := strings.Index(res.HTML, currentMarkID)
	if target < first {
		t.Error("the current match is not the second one")
	}
	second := strings.Index(res.HTML[first+1:], "<mark") + first + 1
	if target < second {
		t.Errorf("the scroll target landed on the first match: %s", res.HTML)
	}
}

// Past either end the search comes round, because a find bar that stopped dead
// would make the reader scroll back by hand.
func TestTheCurrentMatchWrapsAtBothEnds(t *testing.T) {
	const body = "<p>a b a b a</p>"

	if got := mustHighlight(t, body, "a", 3).Current; got != 0 {
		t.Errorf("index 3 of 3 landed on %d, want 0", got)
	}
	if got := mustHighlight(t, body, "a", -1).Current; got != 2 {
		t.Errorf("index -1 landed on %d, want the last match", got)
	}
}

// Case-insensitive, or searching for a word at the start of a sentence would
// need the reader to guess how the sender capitalised it.
func TestTheSearchIgnoresCase(t *testing.T) {
	res := mustHighlight(t, "<p>Fatura FATURA fatura</p>", "fAtUrA", 0)

	if res.Count != 3 {
		t.Errorf("Count = %d, want 3", res.Count)
	}
	// The original capitalisation is kept: the mark wraps the sender's text,
	// it does not replace it with the query.
	for _, want := range []string{">Fatura<", ">FATURA<", ">fatura<"} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("%s is missing from %s", want, res.HTML)
		}
	}
}

// Turkish is where simple case folding shows its edges, and the behaviour is
// worth pinning down rather than discovering in use.
func TestTurkishDottedAndDotlessLetters(t *testing.T) {
	// I and İ both fold to i, so one search finds both spellings.
	if got := mustHighlight(t, "<p>İstanbul ISTANBUL istanbul</p>", "istanbul", 0).Count; got != 3 {
		t.Errorf("searching for istanbul found %d, want 3", got)
	}
	// ı folds to itself, so it is only found by searching for ı.
	if got := mustHighlight(t, "<p>ışık</p>", "ışık", 0).Count; got != 1 {
		t.Errorf("searching for ışık found %d, want 1", got)
	}
	if got := mustHighlight(t, "<p>ışık</p>", "isik", 0).Count; got != 0 {
		t.Errorf("searching for isik found %d in ışık, want 0", got)
	}
}

// Offsets are in bytes while matching is in runes, and getting the two mixed
// up would slice a multi-byte letter in half.
func TestMultiByteTextIsNotCutInHalf(t *testing.T) {
	res := mustHighlight(t, "<p>çağrı güncellemesi çağrı</p>", "çağrı", 0)

	if res.Count != 2 {
		t.Fatalf("Count = %d, want 2", res.Count)
	}
	if strings.Contains(res.HTML, "�") {
		t.Errorf("the text was cut mid-letter: %s", res.HTML)
	}
	if got := strings.Count(res.HTML, ">çağrı</mark>"); got != 2 {
		t.Errorf("%d marks hold the whole word, want 2: %s", got, res.HTML)
	}
}

// A match inside a tag name or an attribute is not a match the reader can see,
// and wrapping one would break the markup around it.
func TestTagsAndAttributesAreNotSearched(t *testing.T) {
	res := mustHighlight(t,
		`<a href="https://example.com/span" title="span">okunur</a>`, "span", 0)

	if res.Count != 0 {
		t.Errorf("Count = %d, want 0 — the word is only in the markup", res.Count)
	}
	if strings.Contains(res.HTML, "<mark") {
		t.Errorf("the markup was highlighted: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `href="https://example.com/span"`) {
		t.Errorf("the link was damaged: %s", res.HTML)
	}
}

// PresentationRich keeps the sender's stylesheet. A mark inserted into a CSS
// rule would corrupt the sheet rather than highlight anything.
func TestStylesheetsAreLeftAlone(t *testing.T) {
	res := mustHighlight(t,
		`<style>.color{color:red}</style><p>color</p>`, "color", 0)

	if res.Count != 1 {
		t.Errorf("Count = %d, want 1 — only the paragraph counts", res.Count)
	}
	if !strings.Contains(res.HTML, "<style>.color{color:red}</style>") {
		t.Errorf("the stylesheet was rewritten: %s", res.HTML)
	}
}

// The sender does not get to decide where "next match" scrolls to.
func TestTheSendersOwnMarkersAreRemoved(t *testing.T) {
	res := mustHighlight(t,
		`<p id="nx-find-current" class="nx-find keep">sahte</p><p>sahte</p>`, "sahte", 0)

	if res.Count != 2 {
		t.Fatalf("Count = %d, want 2", res.Count)
	}
	if got := strings.Count(res.HTML, currentMarkID); got != 1 {
		t.Errorf("%d elements carry the scroll target, want only ours: %s", got, res.HTML)
	}
	// Only our namespace goes; the sender's other classes are none of our
	// business.
	if !strings.Contains(res.HTML, `class="keep"`) {
		t.Errorf("an unrelated class was dropped: %s", res.HTML)
	}
}

// Nothing to search for is not an error, and it must leave the body exactly as
// it was: this is the path every message takes when the find bar is closed.
func TestAnEmptyQueryChangesNothing(t *testing.T) {
	const body = `<p>değişmesin</p>`

	for _, query := range []string{"", "   "} {
		res := mustHighlight(t, body, query, 0)
		if res.HTML != body {
			t.Errorf("query %q rewrote the body: %s", query, res.HTML)
		}
		if res.Count != 0 {
			t.Errorf("query %q reported %d matches", query, res.Count)
		}
	}
}

func TestNoMatchesLeavesTheBodyReadable(t *testing.T) {
	res := mustHighlight(t, "<p>burada yok</p>", "fatura", 0)

	if res.Count != 0 || res.Current != 0 {
		t.Errorf("Count = %d, Current = %d, want 0 and 0", res.Count, res.Current)
	}
	if !strings.Contains(res.HTML, "burada yok") {
		t.Errorf("the body was lost: %s", res.HTML)
	}
}

// Matches spread over several nodes are numbered in reading order, or "next"
// would jump around the message.
func TestMatchesAreNumberedInReadingOrder(t *testing.T) {
	const body = "<p>kod</p><div><span>kod</span> kod</div>"

	res := mustHighlight(t, body, "kod", 2)
	if res.Count != 3 {
		t.Fatalf("Count = %d, want 3", res.Count)
	}
	// The third match is the bare one in the div, after the span.
	tail := res.HTML[strings.Index(res.HTML, "</span>"):]
	if !strings.Contains(tail, currentMarkID) {
		t.Errorf("index 2 did not land on the last match: %s", res.HTML)
	}
}

// Sanitize runs bluemonday last, so Highlight must be able to take its output
// without the marks being stripped or the body changing meaning.
func TestHighlightingSurvivesOnSanitizedOutput(t *testing.T) {
	sanitised, err := Sanitize(
		`<p>Sipariş <b>onayı</b></p><script>alert(1)</script>`, Options{})
	if err != nil {
		t.Fatalf("Sanitize() error: %v", err)
	}

	res := mustHighlight(t, sanitised.HTML, "sipariş", 0)
	if res.Count != 1 {
		t.Errorf("Count = %d, want 1", res.Count)
	}
	if !strings.Contains(res.HTML, "<mark") {
		t.Errorf("the match was not wrapped: %s", res.HTML)
	}
	if strings.Contains(res.HTML, "alert(1)") {
		t.Errorf("the script came back: %s", res.HTML)
	}
}
