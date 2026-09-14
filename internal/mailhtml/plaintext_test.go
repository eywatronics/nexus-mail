package mailhtml

import (
	"strings"
	"testing"
)

// "Show me the text" cannot always be answered by reading the message's
// text/plain part: plenty of mail is HTML only, and on exactly that mail the
// reader is most likely to want the decoration gone.
func TestPlainTextKeepsTheWordsAndDropsTheMarkup(t *testing.T) {
	got, err := PlainText(`<div><p>Merhaba <b>Zeynep</b>,</p><p>Mutabakat ektedir.</p></div>`)
	if err != nil {
		t.Fatalf("PlainText() error: %v", err)
	}

	if !strings.Contains(got, "Merhaba Zeynep,") {
		t.Errorf("PlainText() = %q, want the sentence intact", got)
	}
	if strings.Contains(got, "<") {
		t.Errorf("PlainText() = %q, want no markup", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("PlainText() = %q, want the paragraphs on separate lines", got)
	}
}

// A stylesheet's text is markup, not message. Leaving it in would open the
// plain-text view with a wall of CSS — the exact thing the reader switched
// away from.
func TestPlainTextDropsStyleAndScriptContent(t *testing.T) {
	got, err := PlainText(
		`<style>body{color:red}</style><script>alert(1)</script><p>Gövde</p>`)
	if err != nil {
		t.Fatalf("PlainText() error: %v", err)
	}

	if strings.Contains(got, "color:red") || strings.Contains(got, "alert") {
		t.Errorf("PlainText() = %q, want the stylesheet and script gone", got)
	}
	if !strings.Contains(got, "Gövde") {
		t.Errorf("PlainText() = %q, want the body text", got)
	}
}

// A message wrapped in six divs would otherwise open with six blank lines.
func TestPlainTextCollapsesTheBlankLinesNestingProduces(t *testing.T) {
	got, err := PlainText(`<div><div><div><p>Tek satır</p></div></div></div>`)
	if err != nil {
		t.Fatalf("PlainText() error: %v", err)
	}

	if got != "Tek satır" {
		t.Errorf("PlainText() = %q, want exactly the sentence", got)
	}
}

// Two cells running into each other read as one word, which is how a price
// table becomes nonsense.
func TestPlainTextSeparatesTableCells(t *testing.T) {
	got, err := PlainText(`<table><tr><td>Ocak</td><td>1.250</td></tr></table>`)
	if err != nil {
		t.Fatalf("PlainText() error: %v", err)
	}

	if strings.Contains(got, "Ocak1.250") {
		t.Errorf("PlainText() = %q, want the cells separated", got)
	}
}

func TestPlainTextTurnsBreaksIntoLines(t *testing.T) {
	got, err := PlainText(`Bir<br>İki<br>Üç`)
	if err != nil {
		t.Fatalf("PlainText() error: %v", err)
	}

	if got != "Bir\nİki\nÜç" {
		t.Errorf("PlainText() = %q, want three lines", got)
	}
}

func TestPlainTextOfNothingIsNothing(t *testing.T) {
	got, err := PlainText("")
	if err != nil {
		t.Fatalf("PlainText() error: %v", err)
	}
	if got != "" {
		t.Errorf("PlainText(\"\") = %q", got)
	}
}
