package mailhtml

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Find-in-message is done here, in Go, rather than in the frame.
//
// The reading pane is an iframe with neither allow-scripts nor
// allow-same-origin, so the browser's own find is unreachable from the window:
// there is no script inside to run it and no handle from outside to call it.
// That sandbox is the point of the reading pane, so the search comes to the
// markup instead — matches are wrapped before the document is served, and the
// current one is scrolled to with a URL fragment, which needs no script at all.
const (
	// markClass is on every match, so the frame's stylesheet can find them
	// without touching a <mark> the sender wrote.
	markClass = "nx-find"
	// currentMarkID is the scroll target. An id rather than a class because a
	// fragment is the only way to move a scriptless document.
	currentMarkID = "nx-find-current"
	// markerPrefix is the namespace this package claims in the rendered
	// document. Anything of the sender's wearing it is removed.
	markerPrefix = "nx-"
)

// unsearchable elements hold text the reader is not looking at.
//
// style matters most: PresentationRich keeps the sender's stylesheet, and a
// <mark> inserted into a CSS rule would corrupt the sheet rather than highlight
// anything.
var unsearchable = map[string]bool{
	"style": true, "script": true, "title": true, "textarea": true,
}

// HighlightResult is the marked-up body and how many matches it holds.
type HighlightResult struct {
	HTML  string
	Count int
	// Current is the index that was actually marked, after wrapping. The caller
	// asked for one and may have been past either end; this is the answer the
	// document was built with, so the window can show the same number.
	Current int
}

// Highlight wraps every occurrence of query in already-sanitised body HTML.
//
// The input must be the output of Sanitize. bluemonday has already run by then
// and does not run again, which is what lets the <mark> elements survive — and
// is why nothing but our own markup may be added here.
//
// Matches do not cross element boundaries: a word split by a <b> in the middle
// is not found. The browser's own find does better, but reaching it would mean
// giving the frame scripts. That trade is the whole reason this function
// exists, so the limitation is accepted rather than worked around.
func Highlight(sanitised, query string, current int) (HighlightResult, error) {
	if strings.TrimSpace(query) == "" || strings.TrimSpace(sanitised) == "" {
		return HighlightResult{HTML: sanitised}, nil
	}

	// A body context rather than html.Parse: the input is a fragment, and
	// parsing it as a document would wrap it in <html><body> that the caller
	// then nests inside its own.
	holder := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(sanitised), holder)
	if err != nil {
		return HighlightResult{}, fmt.Errorf("mailhtml: parsing for find: %w", err)
	}
	// The fragment's top-level nodes come back parentless. They are hung under
	// the holder because a match in one of them has to be replaceable, and a
	// node replaces itself through its parent.
	for _, n := range nodes {
		holder.AppendChild(n)
	}

	needle := foldRunes(query)
	var found []nodeMatches
	total := 0
	collect(holder, needle, &found, &total)

	if total > 0 {
		// Wrapping both ways, so next from the last match returns to the first
		// and previous from the first reaches the last. A find bar that stopped
		// dead at the ends would make the reader scroll back by hand.
		current = ((current % total) + total) % total

		index := 0
		for _, m := range found {
			markMatches(m.node, m.ranges, index, current)
			index += len(m.ranges)
		}
	} else {
		current = 0
	}

	var buf strings.Builder
	for child := holder.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&buf, child); err != nil {
			return HighlightResult{}, fmt.Errorf("mailhtml: rendering find results: %w", err)
		}
	}
	return HighlightResult{HTML: buf.String(), Count: total, Current: current}, nil
}

// nodeMatches is one text node and where the query occurs in it.
type nodeMatches struct {
	node   *html.Node
	ranges [][2]int
}

// collect walks the tree, strips our namespace from the sender's markup and
// records where the query occurs.
//
// The stripping is not cosmetic. The current match is reached through a
// fragment, and a sender who wrote id="nx-find-current" into their own message
// would otherwise decide where "next match" scrolls to.
func collect(n *html.Node, needle []rune, out *[]nodeMatches, total *int) {
	if n.Type == html.ElementNode {
		if unsearchable[n.Data] {
			return
		}
		dropOurMarkers(n)
	}

	if n.Type == html.TextNode {
		if ranges := foldText(n.Data).find(needle); len(ranges) > 0 {
			*out = append(*out, nodeMatches{node: n, ranges: ranges})
			*total += len(ranges)
		}
		return
	}

	// The children are collected before any of them is replaced: markMatches
	// removes the node it was given, and doing that mid-walk would cut the
	// sibling list the walk is following.
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collect(child, needle, out, total)
	}
}

// dropOurMarkers removes id and class values in this package's namespace, so
// the only ones in the served document are the ones put there below.
func dropOurMarkers(n *html.Node) {
	kept := n.Attr[:0]
	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "id":
			if strings.HasPrefix(attr.Val, markerPrefix) {
				continue
			}
		case "class":
			attr.Val = withoutOurClasses(attr.Val)
			if attr.Val == "" {
				continue
			}
		}
		kept = append(kept, attr)
	}
	n.Attr = kept
}

func withoutOurClasses(value string) string {
	kept := make([]string, 0, 4)
	for name := range strings.FieldsSeq(value) {
		if strings.HasPrefix(name, markerPrefix) {
			continue
		}
		kept = append(kept, name)
	}
	return strings.Join(kept, " ")
}

// markMatches replaces one text node with the alternating run of plain text and
// <mark> elements its matches call for.
func markMatches(n *html.Node, ranges [][2]int, firstIndex, current int) {
	parent := n.Parent
	if parent == nil {
		return
	}

	text := n.Data
	cursor := 0
	for k, r := range ranges {
		if r[0] > cursor {
			parent.InsertBefore(textNode(text[cursor:r[0]]), n)
		}

		mark := &html.Node{Type: html.ElementNode, Data: "mark", DataAtom: atom.Mark,
			Attr: []html.Attribute{{Key: "class", Val: markClass}}}
		if firstIndex+k == current {
			mark.Attr = append(mark.Attr, html.Attribute{Key: "id", Val: currentMarkID})
		}
		mark.AppendChild(textNode(text[r[0]:r[1]]))
		parent.InsertBefore(mark, n)

		cursor = r[1]
	}
	if cursor < len(text) {
		parent.InsertBefore(textNode(text[cursor:]), n)
	}
	parent.RemoveChild(n)
}

func textNode(data string) *html.Node {
	return &html.Node{Type: html.TextNode, Data: data}
}

// folded is a string prepared for case-insensitive search, with a way back to
// byte offsets in the original.
//
// Folding is per rune and always produces exactly one rune, so the mapping
// stays one to one. Full Unicode case folding does not have that property —
// German ß folds to two letters — and a mapping that changed length would put
// the <mark> elements in the wrong place, which is worse than missing a match.
type folded struct {
	runes []rune
	// offsets holds the byte offset of each rune, plus a final sentinel at the
	// end of the string, so a match ending on the last rune has somewhere to
	// read its end offset from.
	offsets []int
}

// foldText prepares a haystack. Turkish is the case this is worst at: I and İ
// both fold to i, so searching for "i" finds both, while ı folds to itself and
// is only found by searching for ı. That is the behaviour of simple case
// folding, and getting it properly right needs a locale the reading pane does
// not have.
func foldText(s string) folded {
	f := folded{
		runes:   make([]rune, 0, len(s)),
		offsets: make([]int, 0, len(s)+1),
	}
	for i, r := range s {
		f.offsets = append(f.offsets, i)
		f.runes = append(f.runes, unicode.ToLower(r))
	}
	f.offsets = append(f.offsets, len(s))
	return f
}

func foldRunes(s string) []rune {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		out = append(out, unicode.ToLower(r))
	}
	return out
}

// find returns the byte ranges of every non-overlapping occurrence.
func (f folded) find(needle []rune) [][2]int {
	if len(needle) == 0 || len(f.runes) < len(needle) {
		return nil
	}

	var out [][2]int
	for i := 0; i+len(needle) <= len(f.runes); {
		if !matchesAt(f.runes[i:], needle) {
			i++
			continue
		}
		out = append(out, [2]int{f.offsets[i], f.offsets[i+len(needle)]})
		i += len(needle)
	}
	return out
}

func matchesAt(haystack, needle []rune) bool {
	for i, r := range needle {
		if haystack[i] != r {
			return false
		}
	}
	return true
}
