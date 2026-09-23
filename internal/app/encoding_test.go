package app

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// latin5Message is a mail whose header claims UTF-8 and whose bytes are not:
// "Mutabakat sözleşmesi" in ISO-8859-9. This is the shape of the problem the
// repair exists for — Turkish corporate systems still send it, and nothing in
// the normal path can fix it, because the normal path is doing what it was
// told.
func latin5Message() []byte {
	var b bytes.Buffer
	b.WriteString("Subject: Rapor\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.Write([]byte{'M', 'u', 't', 'a', 'b', 'a', 'k', 'a', 't', ' ',
		's', 0xF6, 'z', 'l', 'e', 0xFE, 'm', 'e', 's', 'i'})
	return b.Bytes()
}

const repairedTurkish = "Mutabakat sözleşmesi"

// countingCache records the message ids whose render was dropped.
type countingCache struct{ dropped []int64 }

func (c *countingCache) Invalidate(messageID int64) {
	c.dropped = append(c.dropped, messageID)
}

func TestRepairEncodingRewritesTheStoredBody(t *testing.T) {
	svc, _, db := newTestService(t, rawBackend{raw: latin5Message()})
	id := firstMessage(t, svc)

	if err := svc.RepairEncoding(id, "iso-8859-9"); err != nil {
		t.Fatalf("RepairEncoding() error: %v", err)
	}

	// Stored, not applied for one render. A reader who worked out that a
	// correspondent's system lies about its encoding should not have to work
	// it out again every time they open the same message.
	_, text, err := db.GetMessageBody(context.Background(), id)
	if err != nil {
		t.Fatalf("GetMessageBody() error: %v", err)
	}
	if text != repairedTurkish {
		t.Errorf("stored body = %q, want %q", text, repairedTurkish)
	}
}

// The render cache is keyed on message id and knows nothing about encodings.
// Without dropping it the reading pane would go on serving the mojibake the
// reader just fixed.
func TestRepairEncodingDropsTheRenderedCopy(t *testing.T) {
	svc, _, _ := newTestService(t, rawBackend{raw: latin5Message()})
	cache := &countingCache{}
	svc.cfg.InvalidateBody = cache.Invalidate
	id := firstMessage(t, svc)

	if err := svc.RepairEncoding(id, "iso-8859-9"); err != nil {
		t.Fatalf("RepairEncoding() error: %v", err)
	}

	if len(cache.dropped) != 1 || cache.dropped[0] != id {
		t.Errorf("dropped %v, want exactly message %d", cache.dropped, id)
	}
}

// A failed repair must leave the previous body alone. Replacing it with
// nothing would turn a message that merely looked wrong into one that is gone.
func TestAFailedRepairLeavesTheBodyAlone(t *testing.T) {
	svc, _, db := newTestService(t, rawBackend{raw: latin5Message()})
	id := firstMessage(t, svc)

	if err := svc.RepairEncoding(id, "iso-8859-9"); err != nil {
		t.Fatalf("first RepairEncoding() error: %v", err)
	}
	if err := svc.RepairEncoding(id, "not-a-charset"); err == nil {
		t.Fatal("RepairEncoding() accepted a charset that does not exist")
	}

	_, text, err := db.GetMessageBody(context.Background(), id)
	if err != nil {
		t.Fatalf("GetMessageBody() error: %v", err)
	}
	if text != repairedTurkish {
		t.Errorf("stored body = %q after a failed repair, want the previous one", text)
	}
}

// Every entry the picker offers has to name an encoding the repair can
// actually perform, and read as something a person recognises: almost nobody
// looking at a mangled message knows it is Latin-5, plenty know it is Turkish.
func TestTheCharsetPickerOffersWorkingChoices(t *testing.T) {
	svc, _, _ := newTestService(t, rawBackend{raw: latin5Message()})
	id := firstMessage(t, svc)

	options := svc.RepairCharsets()
	if len(options) == 0 {
		t.Fatal("the picker offers nothing")
	}
	if options[0].Name != "utf-8" {
		t.Errorf("the first option is %q; the commonest repair is back to UTF-8", options[0].Name)
	}

	for _, opt := range options {
		if opt.Label == "" || opt.Label == opt.Name {
			t.Errorf("%q has no readable label", opt.Name)
		}
		if strings.TrimSpace(opt.Name) == "" {
			t.Error("an option has no name to repair with")
		}
		if err := svc.RepairEncoding(id, opt.Name); err != nil {
			t.Errorf("the picker offers %q but repairing with it fails: %v", opt.Name, err)
		}
	}
}

// The hook is optional, and a repair must not fall over when nothing set it.
func TestRepairWorksBeforeACacheIsRegistered(t *testing.T) {
	svc, _, _ := newTestService(t, rawBackend{raw: latin5Message()})
	id := firstMessage(t, svc)

	if err := svc.RepairEncoding(id, "iso-8859-9"); err != nil {
		t.Fatalf("RepairEncoding() error: %v", err)
	}
}

// The end of the chain: after a repair, the reading pane has to actually show
// the repaired text. This is the assertion that would catch a cache drop that
// silently did nothing.
func TestTheReadingPaneServesTheRepairedBody(t *testing.T) {
	svc, _, _ := newTestService(t, rawBackend{raw: latin5Message()})
	h := NewBodyHandler(svc)
	svc.cfg.InvalidateBody = h.Invalidate
	id := firstMessage(t, svc)

	// Render once so there is a cached copy of the mangled version to drop.
	before := get(t, h, fmt.Sprintf("%s%d", bodyPath, id)).Body.String()
	if strings.Contains(before, repairedTurkish) {
		t.Fatalf("precondition failed: the message already reads correctly:\n%s", before)
	}

	if err := svc.RepairEncoding(id, "iso-8859-9"); err != nil {
		t.Fatalf("RepairEncoding() error: %v", err)
	}

	after := get(t, h, fmt.Sprintf("%s%d", bodyPath, id)).Body.String()
	if !strings.Contains(after, repairedTurkish) {
		t.Errorf("the pane still serves the old render:\n%s", after)
	}
}
