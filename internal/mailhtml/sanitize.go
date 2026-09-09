// Package mailhtml prepares untrusted mail HTML for display.
//
// This is one of two layers, not the whole defence. Sanitising removes what we
// can recognise; the sandboxed iframe the caller renders into contains what we
// cannot. Neither is sufficient alone.
package mailhtml

import (
	"crypto/sha256"
	"encoding/hex"
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

	// hostScheme matches the schemes Wails serves the app on, across
	// platforms. A sender writing one could point the frame at another
	// message's body.
	hostScheme = regexp.MustCompile(`(?i)^\s*(?:wails:|https?://wails\.localhost)`)
)

// blockedPrefix marks an attribute we removed, so the UI can restore it when
// the user consents without refetching the message.
const blockedPrefix = "data-nexus-blocked-"

// Mode selects what happens to remote resource references.
type Mode int

const (
	// ModeBlock strips every remote reference. The default, and what a message
	// gets until the user asks otherwise.
	ModeBlock Mode = iota
	// ModeProxy rewrites remote references to local proxy URLs, so the host
	// fetches them instead of the renderer. The sender learns nothing about
	// the reader: no IP address, no Referer, no user agent.
	ModeProxy
)

// Result reports the sanitised HTML and what it did with remote references.
type Result struct {
	HTML string
	// BlockedRemoteCount drives the "N remote resources blocked" prompt. It is
	// zero in ModeProxy, where nothing is withheld.
	BlockedRemoteCount int
	// RemoteURLs maps each proxy token to the original URL it stands for.
	// Populated only in ModeProxy; the caller needs it to serve the proxy
	// requests the rewritten HTML will make.
	RemoteURLs map[string]string
}

// ProxyURLFunc builds the local URL that stands in for a remote one. The token
// it receives identifies the resource within this message.
type ProxyURLFunc func(token string) string

// Options configures one sanitising pass.
type Options struct {
	Mode Mode
	// ProxyURL is required in ModeProxy.
	ProxyURL ProxyURLFunc
}

// Sanitize cleans raw mail HTML.
//
// In ModeBlock the renderer has nothing to request even if it wanted to: no
// fetching attribute survives, so the browser is never given the opportunity.
func Sanitize(raw string, opts Options) (Result, error) {
	if strings.TrimSpace(raw) == "" {
		return Result{}, nil
	}
	if opts.Mode == ModeProxy && opts.ProxyURL == nil {
		return Result{}, fmt.Errorf("mailhtml: ModeProxy requires a ProxyURL function")
	}

	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return Result{}, fmt.Errorf("mailhtml: parsing message: %w", err)
	}

	st := &state{opts: opts, remote: map[string]string{}}
	walk(doc, st)

	var buf strings.Builder
	if err := html.Render(&buf, doc); err != nil {
		return Result{}, fmt.Errorf("mailhtml: rendering message: %w", err)
	}

	res := Result{
		// bluemonday runs last as a safety net over the whitelist of tags and
		// attributes. The walk above handles what a whitelist cannot express —
		// rewriting a URL rather than dropping the element that carried it.
		HTML:               policy().Sanitize(buf.String()),
		BlockedRemoteCount: st.blocked,
	}
	if opts.Mode == ModeProxy {
		res.RemoteURLs = st.remote
	}
	return res, nil
}

// state carries the per-pass bookkeeping so the walk does not need five
// parameters.
type state struct {
	opts    Options
	blocked int
	remote  map[string]string
}

// rewrite returns what a remote URL should become, and whether it was
// neutralised rather than replaced.
func (s *state) rewrite(original string) (replacement string, blocked bool) {
	if s.opts.Mode == ModeProxy {
		token := tokenFor(original)
		s.remote[token] = original
		return s.opts.ProxyURL(token), false
	}
	s.blocked++
	return "", true
}

// tokenFor derives a stable, opaque identifier for a URL. A hash rather than
// the URL itself keeps the address out of the rewritten document, so a proxy
// URL cannot be read as a tracking address by anything that inspects the DOM.
func tokenFor(url string) string {
	sum := sha256.Sum256([]byte(url))
	return hex.EncodeToString(sum[:16])
}

func walk(n *html.Node, st *state) {
	// Collect first: removing a child while ranging over the sibling list
	// truncates the walk.
	var toRemove []*html.Node

	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && droppedElements[child.Data] {
			toRemove = append(toRemove, child)
			continue
		}
		walk(child, st)
	}
	for _, child := range toRemove {
		n.RemoveChild(child)
	}

	if n.Type != html.ElementNode {
		return
	}
	cleanElement(n, st)
	if n.Data == "style" {
		cleanStyleElement(n, st)
	}
}

func cleanElement(n *html.Node, st *state) {
	kept := n.Attr[:0]

	for _, attr := range n.Attr {
		name := strings.ToLower(attr.Key)

		// Inline handlers are executable code wearing an attribute's syntax.
		if strings.HasPrefix(name, "on") {
			continue
		}
		// A form that can submit is a phishing tool, and mail has no
		// legitimate need to post anywhere.
		if n.Data == "form" && (name == "action" || name == "method") {
			continue
		}

		// The host scheme is ours to emit, never the message's to ask for. A
		// wails: URL written by a sender could point the iframe at another
		// message's body; only the rewrites below are allowed to produce one.
		if hostScheme.MatchString(attr.Val) {
			continue
		}

		if isFetching(name) && remoteURL.MatchString(attr.Val) {
			// srcset lists several candidates, each needing its own decision.
			if name == "srcset" {
				rewritten, dropped := rewriteSrcset(attr.Val, st)
				if dropped {
					kept = append(kept, html.Attribute{Key: blockedPrefix + name, Val: attr.Val})
					continue
				}
				attr.Val = rewritten
				kept = append(kept, attr)
				continue
			}

			replacement, blocked := st.rewrite(attr.Val)
			if blocked {
				kept = append(kept, html.Attribute{Key: blockedPrefix + name, Val: attr.Val})
				continue
			}
			attr.Val = replacement
			kept = append(kept, attr)
			continue
		}

		if name == "style" {
			attr.Val = rewriteCSSURLs(attr.Val, st)
		}
		kept = append(kept, attr)
	}
	n.Attr = kept
}

// rewriteSrcset handles the comma-separated candidate list. In ModeBlock the
// whole attribute goes, since a partially rewritten srcset would still leak
// through whichever candidate survived.
func rewriteSrcset(value string, st *state) (string, bool) {
	if st.opts.Mode == ModeBlock {
		st.blocked++
		return "", true
	}

	parts := strings.Split(value, ",")
	for i, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}
		if remoteURL.MatchString(fields[0]) {
			replacement, _ := st.rewrite(fields[0])
			fields[0] = replacement
		}
		parts[i] = strings.Join(fields, " ")
	}
	return strings.Join(parts, ", "), false
}

func cleanStyleElement(n *html.Node, st *state) {
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.TextNode {
			continue
		}
		child.Data = rewriteCSSURLs(child.Data, st)
	}
}

// rewriteCSSURLs neutralises or proxies remote url() references in a
// stylesheet or a style attribute.
func rewriteCSSURLs(css string, st *state) string {
	return cssURL.ReplaceAllStringFunc(css, func(match string) string {
		groups := cssURL.FindStringSubmatch(match)
		// Exactly one of the three quoting alternatives captured the URL.
		for _, candidate := range groups[1:] {
			if candidate == "" {
				continue
			}
			// Same rule as for attributes: the host scheme is ours to emit,
			// never the message's to ask for.
			if hostScheme.MatchString(candidate) {
				return "url()"
			}
			if !remoteURL.MatchString(candidate) {
				continue
			}
			replacement, blocked := st.rewrite(candidate)
			if blocked {
				return "url()"
			}
			return "url(\"" + replacement + "\")"
		}
		return match
	})
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

	// cid: refers to a part already inside the message, so resolving it is not
	// a network fetch. Proxied images are rewritten to ordinary http(s) URLs
	// on the app's own origin, which is why no bespoke scheme is listed here.
	p.AllowURLSchemes("http", "https", "mailto", "tel", "cid", "data")
	p.AllowAttrs("src", "alt", "title", "srcset").OnElements("img")
	p.RequireNoFollowOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	return p
}
