package imapx

import (
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"

	"nexusmail/internal/model"
)

// replyPrefixes are stripped when deriving a thread key from a subject.
//
// Turkish Outlook uses YNT (yanıt) and İLT (ilet); without them a single
// corporate Turkish conversation splits into two threads. The others cover the
// common European clients.
// Entries must be written as strings.ToLower would produce them. Turkish "İLT"
// lowercases to "i" followed by a combining dot above, not to a plain "i",
// which is why the Turkish entries look doubled here.
var replyPrefixes = []string{
	"re:", "fwd:", "fw:", // English
	"ynt:", "yan:", "ilt:", "i̇lt:", // Turkish
	"aw:", "wg:", // German
	"sv:", "vs:", // Nordic
	"rif:", // Italian
}

// maxDateSkew is how far ahead of INTERNALDATE a Date: header may legitimately
// sit. Real mail drifts by hours through timezone mistakes; more than two days
// ahead is either broken or deliberate, and in both cases the server's
// delivery time is the better answer.
const maxDateSkew = 48 * time.Hour

// ThreadKey derives a stable conversation identifier.
//
// Preference order follows the simplified JWZ approach: the References root,
// then In-Reply-To, then the message's own ID, and finally a normalised subject
// for servers and gateways that strip Message-ID entirely.
func ThreadKey(messageID, inReplyTo string, references []string, subject string) string {
	if len(references) > 0 && references[0] != "" {
		return references[0]
	}
	if inReplyTo != "" {
		// In-Reply-To can carry several ids; the first is the direct parent.
		if fields := strings.Fields(inReplyTo); len(fields) > 0 {
			return fields[0]
		}
	}
	if messageID != "" {
		return messageID
	}
	return "subject:" + normaliseSubject(subject)
}

func normaliseSubject(s string) string {
	// Lowercase FIRST, then strip. Slicing the original string by the length
	// of a lowercased prefix is a trap in Turkish: strings.ToLower("İ")
	// produces "i" plus a combining dot above, so the lowercase form is a byte
	// longer than what it came from and the slice cuts in the wrong place.
	// Working on one string throughout removes the mismatch entirely.
	out := strings.TrimSpace(strings.ToLower(s))

	// Strip repeatedly: "Re: Fwd: Re: x" is one conversation, not three.
	for changed := true; changed; {
		changed = false
		for _, p := range replyPrefixes {
			if strings.HasPrefix(out, p) {
				out = strings.TrimSpace(out[len(p):])
				changed = true
				break
			}
		}
	}
	// Collapse whitespace so wrapped subjects match their unwrapped form.
	return strings.Join(strings.Fields(out), " ")
}

// reconcileDate picks a trustworthy timestamp.
//
// The RFC 2822 Date: header is attacker-controlled and frequently malformed —
// spam sends things like "Pzt, 99 Xyz 2026 29:99:99" — so INTERNALDATE wins
// whenever the header is unusable or implausible.
func reconcileDate(headerDate, internalDate time.Time) time.Time {
	if internalDate.IsZero() {
		// Nothing better to fall back to.
		return headerDate
	}
	if headerDate.IsZero() {
		return internalDate
	}
	if headerDate.After(internalDate.Add(maxDateSkew)) {
		return internalDate
	}
	return headerDate
}

func addressesFrom(list []imap.Address) []model.Address {
	if len(list) == 0 {
		return nil
	}
	out := make([]model.Address, 0, len(list))
	for _, a := range list {
		out = append(out, model.Address{Name: a.Name, Addr: a.Addr()})
	}
	return out
}

func firstAddress(list []imap.Address) model.Address {
	if len(list) == 0 {
		return model.Address{}
	}
	return model.Address{Name: list[0].Name, Addr: list[0].Addr()}
}

// hasAttachmentParts walks a BODYSTRUCTURE looking for a part disposed as an
// attachment, or a non-text leaf part. Inline images in HTML mail count, which
// matches what users expect the paperclip to mean.
func hasAttachmentParts(bs imap.BodyStructure) bool {
	found := false
	bs.Walk(func(_ []int, part imap.BodyStructure) bool {
		if found {
			return false
		}
		single, ok := part.(*imap.BodyStructureSinglePart)
		if !ok {
			return true
		}
		if disp := single.Disposition(); disp != nil &&
			strings.EqualFold(disp.Value, "attachment") {
			found = true
			return false
		}
		if !strings.EqualFold(single.Type, "text") {
			found = true
			return false
		}
		return true
	})
	return found
}
