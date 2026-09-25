package mailmime

import (
	"fmt"
	"strings"
	"time"
)

// replyPrefix is the one RFC 5322 names.
//
// Not a translated one. "Yanıt:" and "AW:" and "Antwort:" all exist in the
// wild, and each one a client invents is another prefix every other client
// fails to recognise — so a thread that crosses two languages grows
// "Re: AW: Re: AW:" down the subject line. Recognising the others and writing
// only this one is what keeps that from happening here.
const replyPrefix = "Re: "

// forwardPrefix, for the same reason.
const forwardPrefix = "Fwd: "

// knownReplyPrefixes are the ones seen often enough to strip.
//
// Stripped rather than kept, because the point of the prefix is to say "this
// is a reply" once. A second copy says it twice and a fifth says nothing at
// all except that five clients disagreed about the wording.
var knownReplyPrefixes = []string{"re:", "re :", "aw:", "antwort:", "yanıt:", "yan:", "sv:", "vs:", "ref:"}

var knownForwardPrefixes = []string{"fwd:", "fw:", "wg:", "ilt:", "vs:", "tr:"}

// ReplySubject is the subject a reply carries.
func ReplySubject(original string) string {
	return replyPrefix + stripPrefixes(original, knownReplyPrefixes)
}

// ForwardSubject is the subject a forward carries.
func ForwardSubject(original string) string {
	return forwardPrefix + stripPrefixes(original, append(append([]string{},
		knownForwardPrefixes...), knownReplyPrefixes...))
}

// stripPrefixes removes every leading marker, in any order and any case.
func stripPrefixes(subject string, prefixes []string) string {
	s := strings.TrimSpace(subject)

	for changed := true; changed; {
		changed = false
		lower := strings.ToLower(s)
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				s = strings.TrimSpace(s[len(p):])
				changed = true
				break
			}
		}
	}
	return s
}

// Quote wraps the original message for a reply.
//
// The attribution line first, then the body with a marker on every line. The
// marker is "> " and not an indent or a coloured bar: it survives being quoted
// again, it survives plain text, and it is what every other client writes — a
// reply quoted three deep should read as three levels everywhere, not as a
// wall of text in anything that did not invent the same decoration.
//
// Empty lines get the marker too. A blank line without one reads to the next
// client as the end of the quote, which is how a quoted paragraph turns into
// half a quoted paragraph and half new text attributed to the wrong person.
func Quote(author string, when time.Time, body string) string {
	var b strings.Builder
	b.WriteString(attribution(author, when))
	b.WriteString("\n")

	for line := range strings.SplitSeq(normaliseToLF(body), "\n") {
		b.WriteString(">")
		if line != "" {
			b.WriteString(" ")
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// attribution is the "On … wrote:" line.
//
// The date is written in the reader's own zone and format rather than the
// sender's, because the person about to read it is the one replying.
func attribution(author string, when time.Time) string {
	if author == "" {
		author = "someone"
	}
	if when.IsZero() {
		return fmt.Sprintf("%s wrote:", author)
	}
	return fmt.Sprintf("On %s, %s wrote:", when.Format("2 January 2006 at 15:04"), author)
}

// ReplyReferences is the References header a reply carries.
//
// The parent's References plus the parent's own Message-ID, which is what
// threads a conversation in every other client. Capped, because a long thread
// otherwise grows a header that some servers refuse: RFC 5322 says to keep the
// first and drop from the middle, since the root is what identifies the thread
// and the recent ones are what identify the position in it.
func ReplyReferences(parentReferences []string, parentMessageID string) []string {
	refs := append(append([]string{}, parentReferences...), parentMessageID)

	// Deduplicated: a client that already appended its own id leaves it here
	// twice, and a References header that repeats itself is one more thing for
	// a threading algorithm to trip over.
	seen := map[string]bool{}
	unique := refs[:0]
	for _, r := range refs {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		unique = append(unique, r)
	}

	const maxReferences = 20
	if len(unique) <= maxReferences {
		return unique
	}
	// The root, then the most recent. The middle is what goes.
	out := append([]string{unique[0]}, unique[len(unique)-(maxReferences-1):]...)
	return out
}

func normaliseToLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
