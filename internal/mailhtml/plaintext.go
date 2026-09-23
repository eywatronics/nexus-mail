package mailhtml

import (
	"strings"

	"golang.org/x/net/html"
)

// blockElements end a line when the walk leaves them, so a plain-text render
// of an HTML message has paragraphs instead of one run-on line.
var blockElements = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"div": true, "dd": true, "dl": true, "dt": true, "fieldset": true,
	"figcaption": true, "figure": true, "footer": true, "form": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"header": true, "hr": true, "li": true, "main": true, "nav": true,
	"ol": true, "p": true, "pre": true, "section": true, "table": true,
	"tr": true, "ul": true,
}

// PlainText reduces mail HTML to its text.
//
// Needed because "show me the plain text" cannot always be answered by reading
// the message's text/plain part: plenty of mail is sent as HTML only, and on
// exactly that mail the reader is most likely to want the decoration gone.
// Falling back to the HTML render when they asked for text would be answering
// a different question.
//
// This is a readability transform, not a security one. The result is escaped
// and shown inside a <pre>; nothing here is load-bearing for safety.
func PlainText(raw string) (string, error) {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return "", err
	}

	var b strings.Builder
	collectText(doc, &b)
	return squeezeBlankLines(b.String()), nil
}

func collectText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type == html.ElementNode {
		switch n.Data {
		case "script", "style", "head", "title":
			// Their text is markup, not message.
			return
		case "br":
			b.WriteString("\n")
			return
		case "td", "th":
			// A cell boundary is a space at minimum, or two columns run into
			// each other and read as one word.
			defer b.WriteString("\t")
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, b)
	}

	if n.Type == html.ElementNode && blockElements[n.Data] {
		b.WriteString("\n")
	}
}

// squeezeBlankLines collapses the runs of empty lines that nested block
// elements produce. A message wrapped in six divs would otherwise open with
// six blank lines.
func squeezeBlankLines(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")

	out := make([]string, 0, len(lines))
	blank := 0
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" {
			blank++
			if blank > 1 || len(out) == 0 {
				continue
			}
			out = append(out, "")
			continue
		}
		blank = 0
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
