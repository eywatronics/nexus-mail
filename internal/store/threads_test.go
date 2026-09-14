package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

// threadMessage is one message in a conversation, at a given minute.
func threadMessage(acct, folder int64, uid uint32, thread, subject string, minute int) model.Message {
	return model.Message{
		AccountID: acct, FolderID: folder, UID: uid,
		MessageID: thread, ThreadID: thread, Subject: subject,
		InternalDate: time.Unix(int64(minute)*60, 0),
	}
}

func seedConversations(t *testing.T, s *Store, acct, inbox int64) {
	t.Helper()

	// Two conversations, interleaved in time on purpose: a list that sorted
	// purely by date would alternate between them.
	msgs := []model.Message{
		threadMessage(acct, inbox, 1, "<mutabakat@x>", "Mutabakat", 10),
		threadMessage(acct, inbox, 2, "<bordro@x>", "Bordro", 20),
		threadMessage(acct, inbox, 3, "<mutabakat@x>", "Re: Mutabakat", 30),
		threadMessage(acct, inbox, 4, "<bordro@x>", "Re: Bordro", 40),
		threadMessage(acct, inbox, 5, "<mutabakat@x>", "Re: Mutabakat", 50),
	}
	if err := s.UpsertMessages(context.Background(), inbox, msgs); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
}

// A conversation is one thing to the reader even though it is many rows here.
// The window groups runs of adjacent rows, so keeping them adjacent is the
// query's job.
func TestThreadedListKeepsAConversationTogether(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedConversations(t, s, acct, inbox)

	got, err := s.ListThreadedMessages(ctx, inbox, 50, 0)
	if err != nil {
		t.Fatalf("ListThreadedMessages() error: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d messages, want 5", len(got))
	}

	var order []string
	for _, m := range got {
		order = append(order, m.ThreadID)
	}
	want := []string{"<mutabakat@x>", "<mutabakat@x>", "<mutabakat@x>", "<bordro@x>", "<bordro@x>"}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("thread order = %v, want each conversation contiguous: %v", order, want)
		}
	}
}

// A conversation that gets a reply today has to come back to the top: that is
// what "there is something new here" looks like. Ranking by the thread's first
// message would bury it under everything newer.
func TestThreadsAreRankedByTheirNewestMessage(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedConversations(t, s, acct, inbox)

	got, _ := s.ListThreadedMessages(ctx, inbox, 50, 0)

	// Mutabakat started first but was last replied to, so it leads.
	if got[0].ThreadID != "<mutabakat@x>" {
		t.Errorf("the list opens with %q, want the conversation with the newest reply",
			got[0].ThreadID)
	}
}

// Within a conversation the messages read in the order they were sent, the way
// a conversation is read.
func TestMessagesInAThreadAreOldestFirst(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedConversations(t, s, acct, inbox)

	got, _ := s.ListThreadedMessages(ctx, inbox, 50, 0)

	var uids []uint32
	for _, m := range got[:3] {
		uids = append(uids, m.UID)
	}
	if uids[0] != 1 || uids[1] != 3 || uids[2] != 5 {
		t.Errorf("the conversation reads %v, want it in the order it happened", uids)
	}
}

// The window needs the total before the rest of the conversation arrives, or
// a collapsed thread cannot say how many messages it stands for.
func TestThreadedListReportsHowManyMessagesEachConversationHas(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedConversations(t, s, acct, inbox)

	got, _ := s.ListThreadedMessages(ctx, inbox, 50, 0)

	counts := map[string]int{}
	for _, m := range got {
		counts[m.ThreadID] = m.ThreadCount
	}
	if counts["<mutabakat@x>"] != 3 || counts["<bordro@x>"] != 2 {
		t.Errorf("thread counts = %v, want 3 and 2", counts)
	}
}

// The count has to be the first page's answer too, not something that only
// becomes right once everything is loaded.
func TestTheThreadCountIsRightOnAPartialPage(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedConversations(t, s, acct, inbox)

	got, err := s.ListThreadedMessages(ctx, inbox, 2, 0)
	if err != nil {
		t.Fatalf("ListThreadedMessages() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d messages, want the page of 2", len(got))
	}
	if got[0].ThreadCount != 3 {
		t.Errorf("ThreadCount = %d on a page holding 2 of them, want 3", got[0].ThreadCount)
	}
}

// Paging is by message, so a page can end mid-conversation. The order is what
// guarantees the rest of it arrives first on the next page.
func TestThePageBoundaryDoesNotReorderAConversation(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedConversations(t, s, acct, inbox)

	first, _ := s.ListThreadedMessages(ctx, inbox, 2, 0)
	second, _ := s.ListThreadedMessages(ctx, inbox, 2, 2)

	joined := append(append([]model.Message{}, first...), second...)
	for i, m := range joined {
		if i < 3 && m.ThreadID != "<mutabakat@x>" {
			t.Errorf("position %d across the page boundary is %q, want the conversation continued",
				i, m.ThreadID)
		}
	}
}

// The flat list is what the unthreaded view shows, and it must stay strictly
// newest first — grouping is a display choice, not a change to what "newest"
// means.
func TestTheFlatListIsStillStrictlyNewestFirst(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)
	seedConversations(t, s, acct, inbox)

	got, err := s.ListMessages(ctx, inbox, 50, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	for i := 1; i < len(got); i++ {
		if got[i].InternalDate.After(got[i-1].InternalDate) {
			t.Fatalf("the flat list is out of order at %d: %v then %v",
				i, got[i-1].InternalDate, got[i].InternalDate)
		}
	}
	if got[0].ThreadCount != 0 {
		t.Errorf("the flat list reports ThreadCount %d; it was not asked for", got[0].ThreadCount)
	}
}
