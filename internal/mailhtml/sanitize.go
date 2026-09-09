// Package mailhtml prepares untrusted mail HTML for display.
//
// This is one of two layers, not the whole defence. Sanitising removes what we
// can recognise; the sandboxed iframe the caller renders into contains what we
// cannot. Neither is sufficient alone.
package mailhtml

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// fetchingAttrs cause a browser to request a resource while rendering.
//
// Blocking only img/src is the classic mistake: background, poster and srcset
// all still phone home, which is all a tracking pixel needs.
var fetchingAttrs = []string{
	"src", "srcset", "poster", "background", "data-src", "data-srcset", "lowsrc",
}

// droppedElements are removed with their contents. Mail is a document, not an
// application.
var droppedElements = map[string]bool{
	"script": true, "iframe": true, "object": true, "embed": true,
	"applet": true, "link": true, "meta": true, "base": true, "noscript": true,
}

var (
	remoteURL = regexp.MustCompile(`^\s*(?:https?:)?//`)

	// Go's regexp engine is RE2, which has no backreferences, so the three
	// quoting styles are spelled out rather than captured and matched.
	cssURL = regexp.MustCompile(`(?i)url\(\s*(?:'([^']*)'|"([^"]*)"|([^'")\s]*))\s*\)`)
)

// blockedPrefix marks an attribute we removed, so the UI can restore it when
// the user consents without refetching the message.
const blockedPrefix = "data-nexus-blocked-"

// Result reports the sanitised HTML and how much was withheld.
type Result struct {
	HTML string
	// BlockedRemoteCount drives the "N remote resources blocked" prompt.
	BlockedRemoteCount int
}

// Sanitize cleans raw mail HTML. When allowRemote is false every remote
// resource reference is stripped, so the renderer has nothing to request even
// if it wanted to.
func Sanitize(raw string, allowRemote bool) (Result, error) {
	if strings.TrimSpace(raw) == "" {
		return Result{}, nil
	}

	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return Result{}, fmt.Errorf("mailhtml: parsing message: %w", err)
	}

	blocked := 0
	walk(doc, allowRemote, &blocked)

	var buf strings.Builder
	if err := html.Render(&buf, doc); err != nil {
		return Result{}, fmt.Errorf("mailhtml: rendering message: %w", err)
	}

	// bluemonday runs last as a safety net over the whitelist of tags and
	// attributes. The walk above handles what a whitelist cannot express —
	// rewriting a URL rather than dropping the element that carried it.
	return Result{
		HTML:               policy().Sanitize(buf.String()),
		BlockedRemoteCount: blocked,
	}, nil
}

func walk(n *html.Node, allowRemote bool, blocked *int) {
	// Collect first: removing a child while ranging over the sibling list
	// truncates the walk.
	var toRemove []*html.Node

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && droppedElements[child.Data] {
			toRemove = append(toRemove, child)
			continue
		}
		walk(child, allowRemote, blocked)
	}
	for _, child := range toRemove {
		n.RemoveChild(child)
	}

	if n.Type == html.ElementNode {
		cleanElement(n, allowRemote, blocked)
	}
	if n.Type == html.ElementNode && n.Data == "style" {
		cleanStyleElement(n, allowRemote, blocked)
	}
}

func cleanElement(n *html.Node, allowRemote bool, blocked *int) {
	kept := n.Attr[:0]

	for _, attr := range n.Attr {
		name := strings.ToLower(attr.Key)

		// Inline handlers are executable code with an attribute's syntax.
		if strings.HasPrefix(name, "on") {
			continue
		}
		// A form that can submit is a phishing tool, and mail has no
		// legitimate need to post anywhere.
		if n.Data == "form" && (name == "action" || name == "method") {
			continue
		}
		if isFetching(name) && !allowRemote && remoteURL.MatchString(attr.Val) {
			*blocked++
			kept = append(kept, html.Attribute{
				Key: blockedPrefix + name,
				Val: attr.Val,
			})
			continue
		}
		if name == "style" {
			cleaned, n := stripCSSURLs(attr.Val, allowRemote)
			*blocked += n
			attr.Val = cleaned
		}
		kept = append(kept, attr)
	}
	n.Attr = kept
}

func cleanStyleElement(n *html.Node, allowRemote bool, blocked *int) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.TextNode {
			continue
		}
		cleaned, count := stripCSSURLs(child.Data, allowRemote)
		*blocked += count
		child.Data = cleaned
	}
}

// stripCSSURLs neutralises remote url() references in a stylesheet or style
// attribute, counting what it removed.
func stripCSSURLs(css string, allowRemote bool) (string, int) {
	if allowRemote {
		return css, 0
	}

	count := 0
	out := cssURL.ReplaceAllStringFunc(css, func(match string) string {
		groups := cssURL.FindStringSubmatch(match)
		// Exactly one of the three quoting alternatives captured the URL.
		for _, candidate := range groups[1:] {
			if candidate != "" && remoteURL.MatchString(candidate) {
				count++
				return "url()"
			}
		}
		return match
	})
	return out, count
}

func isFetching(name string) bool {
	return slices.Contains(fetchingAttrs, name)
}

// policy builds the final whitelist.
//
// href is kept: a link is not a fetch, nothing loads until the user clicks,
// and hiding where a link points would make phishing harder to spot rather
// than easier.
func policy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th",
		"style", "center", "font", "big", "small")
	p.AllowAttrs("style").Globally()
	p.AllowAttrs("class", "id", "align", "valign", "width", "height",
		"cellpadding", "cellspacing", "border", "bgcolor", "colspan", "rowspan").Globally()
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowDataAttributes()

	// cid: refers to a part already inside the message, so it is not a network
	// fetch; the renderer resolves it locally.
	p.AllowURLSchemes("http", "https", "mailto", "tel", "cid", "data")
	p.AllowAttrs("src", "alt", "title", "srcset").OnElements("img")
	p.RequireNoFollowOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	return p
}
