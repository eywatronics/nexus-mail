package app

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
)

// findURL builds the body URL the window would ask for with the find bar open.
func findURL(id int64, query string, index int) string {
	params := url.Values{}
	params.Set("find", query)
	params.Set("findIndex", fmt.Sprint(index))
	return fmt.Sprintf("%s%d?%s", bodyPath, id, params.Encode())
}

func TestTheServedBodyCarriesTheHighlights(t *testing.T) {
	h, id := newBodyHandler(t, "<p>Fatura numarası: fatura-2026</p>")

	body := get(t, h, findURL(id, "fatura", 0)).Body.String()

	if got := strings.Count(body, "<mark"); got != 2 {
		t.Errorf("the document holds %d marks, want 2: %s", got, body)
	}
	// The stylesheet has to know how to paint them, or the marks are invisible
	// and the find does nothing the reader can see.
	if !strings.Contains(body, "mark.nx-find{") {
		t.Error("the frame's stylesheet does not style the matches")
	}
	if !strings.Contains(body, "mark#nx-find-current{") {
		t.Error("the frame's stylesheet does not distinguish the current match")
	}
}

// The window cannot read inside the frame, so the count has to be told to it.
func TestTheWindowIsToldHowManyMatchesThereAre(t *testing.T) {
	h, id, rec := newBodyHandlerWithEvents(t, "<p>kod kod kod</p>")

	get(t, h, findURL(id, "kod", 1))

	found, ok := rec.lastFind()
	if !ok {
		t.Fatal("the handler reported no find result")
	}
	if found.MessageID != id {
		t.Errorf("MessageID = %d, want %d", found.MessageID, id)
	}
	if found.Query != "kod" {
		t.Errorf("Query = %q, want kod", found.Query)
	}
	if found.Count != 3 {
		t.Errorf("Count = %d, want 3", found.Count)
	}
	if found.Current != 1 {
		t.Errorf("Current = %d, want 1", found.Current)
	}
}

// The index the window asks for is wrapped here, and the answer has to come
// back: otherwise the bar would show "4 of 3" while the frame showed the first.
func TestAnOutOfRangeIndexIsReportedAfterWrapping(t *testing.T) {
	h, id, rec := newBodyHandlerWithEvents(t, "<p>kod kod kod</p>")

	get(t, h, findURL(id, "kod", 7))

	found, _ := rec.lastFind()
	if found.Current != 1 {
		t.Errorf("Current = %d, want 7 wrapped into 1", found.Current)
	}
}

// A message with no match still has to render. The reader is looking at it.
func TestAMissNeitherBlanksThePaneNorGoesUnreported(t *testing.T) {
	h, id, rec := newBodyHandlerWithEvents(t, "<p>burada yok</p>")

	body := get(t, h, findURL(id, "fatura", 0)).Body.String()

	if !strings.Contains(body, "burada yok") {
		t.Errorf("the message was lost: %s", body)
	}
	if strings.Contains(body, "<mark") {
		t.Errorf("something was highlighted anyway: %s", body)
	}

	found, ok := rec.lastFind()
	if !ok {
		t.Fatal("the handler said nothing about a search that found nothing")
	}
	if found.Count != 0 {
		t.Errorf("Count = %d, want 0", found.Count)
	}
}

// Closing the find bar has to leave the message exactly as it was, and must
// not keep telling the window about a search nobody is running.
func TestWithNoQueryNothingIsHighlightedAndNothingIsAnnounced(t *testing.T) {
	h, id, rec := newBodyHandlerWithEvents(t, "<p>kod</p>")

	plain := get(t, h, fmt.Sprintf("%s%d", bodyPath, id)).Body.String()
	blank := get(t, h, fmt.Sprintf("%s%d?find=%%20", bodyPath, id)).Body.String()

	if strings.Contains(plain, "<mark") || strings.Contains(blank, "<mark") {
		t.Error("a body with no query came back highlighted")
	}
	if plain != blank {
		t.Error("a blank query changed the document")
	}
	if _, ok := rec.lastFind(); ok {
		t.Error("the handler announced a find nobody asked for")
	}
}

// Highlighting runs on the cached render rather than being folded into it.
//
// The sanitising is the expensive half, and a find bar re-runs its query on
// every keystroke: if each one re-rendered the message, typing into the bar
// would be the slowest thing in the app on exactly the messages — long ones —
// where searching is worth doing.
func TestSearchingReusesTheRenderRatherThanRedoingIt(t *testing.T) {
	h, id := newBodyHandler(t, "<p>kod kod</p>")

	get(t, h, fmt.Sprintf("%s%d", bodyPath, id))
	h.mu.Lock()
	before := h.cache[id]
	h.mu.Unlock()

	for i := range 5 {
		get(t, h, findURL(id, "kod", i))
	}

	h.mu.Lock()
	after := h.cache[id]
	h.mu.Unlock()
	if before != after {
		t.Error("the message was rendered again to answer a search")
	}
}

// The find query reaches the frame as markup, so it must not be able to carry
// anything but text — a query is user input arriving through a URL.
func TestAQueryCannotSmuggleMarkupIntoTheFrame(t *testing.T) {
	h, id := newBodyHandler(t, `<p>onay <script>x</script> onay</p>`)

	body := get(t, h, findURL(id, "<img src=x onerror=alert(1)>", 0)).Body.String()

	if strings.Contains(body, "onerror") {
		t.Errorf("the query reached the document as markup: %s", body)
	}
	if strings.Contains(body, "<script") {
		t.Errorf("a script survived into the framed document: %s", body)
	}
}
