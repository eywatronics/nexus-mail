package sync

import (
	"context"
	"testing"

	"nexusmail/internal/model"
)

func TestEnsureBodyFetchesThenServesFromCache(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 7)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()
	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}

	inbox := folderByPath(t, s, acct.ID, "INBOX")
	msgs, err := s.ListMessages(ctx, inbox.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].BodyFetched {
		t.Fatal("precondition failed: the body is already marked as fetched")
	}

	body, err := eng.EnsureBody(ctx, acct, inbox, msgs[0])
	if err != nil {
		t.Fatalf("EnsureBody() error: %v", err)
	}
	if body.HTML != "<p>body 7</p>" {
		t.Errorf("Body.HTML = %q, want <p>body 7</p>", body.HTML)
	}

	// Blank the server's bodies. The second call must still succeed, which is
	// the local-first promise stated as an assertion rather than an intention.
	be.mu.Lock()
	be.bodies = nil
	be.mu.Unlock()

	again, err := eng.EnsureBody(ctx, acct, inbox, msgs[0])
	if err != nil {
		t.Fatalf("second EnsureBody() error: %v", err)
	}
	if again.HTML != "<p>body 7</p>" {
		t.Errorf("cached Body.HTML = %q, want <p>body 7</p>", again.HTML)
	}

	stored, err := s.ListMessages(ctx, inbox.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if !stored[0].BodyFetched {
		t.Error("BodyFetched = false after EnsureBody; the message list cannot tell what is cached")
	}
}

// A cached read must not touch the network at all, not merely avoid the fetch.
// Selecting a mailbox on every body view would keep an idle client chatty.
func TestEnsureBodyDoesNotReconnectForACachedBody(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 7)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()
	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}

	inbox := folderByPath(t, s, acct.ID, "INBOX")
	msgs, _ := s.ListMessages(ctx, inbox.ID, 10, 0)

	if _, err := eng.EnsureBody(ctx, acct, inbox, msgs[0]); err != nil {
		t.Fatalf("EnsureBody() error: %v", err)
	}

	be.mu.Lock()
	selectsAfterFirst := be.selectCalls
	be.mu.Unlock()

	if _, err := eng.EnsureBody(ctx, acct, inbox, msgs[0]); err != nil {
		t.Fatalf("second EnsureBody() error: %v", err)
	}

	be.mu.Lock()
	selectsAfterSecond := be.selectCalls
	be.mu.Unlock()

	if selectsAfterSecond != selectsAfterFirst {
		t.Errorf("the cached read issued %d extra SELECT(s), want 0",
			selectsAfterSecond-selectsAfterFirst)
	}
}

func TestEnsureBodyReportsAMissingMessage(t *testing.T) {
	be := newFakeBackend()
	be.addFolder("INBOX", nil, 100)
	be.addMessages("INBOX", 7)

	eng, s, acct := newTestEngine(t, be)
	ctx := context.Background()
	if err := eng.InitialSync(ctx, acct); err != nil {
		t.Fatalf("InitialSync() error: %v", err)
	}
	inbox := folderByPath(t, s, acct.ID, "INBOX")

	// A UID the server does not have: the message was expunged between the
	// header sync and the user opening it, which happens routinely.
	_, err := eng.EnsureBody(ctx, acct, inbox, model.Message{ID: 999, UID: 4242})
	if err == nil {
		t.Fatal("EnsureBody() succeeded for a message the server does not have")
	}
}
