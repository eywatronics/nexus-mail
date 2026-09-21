package app

import (
	"context"
	"testing"

	"nexusmail/internal/model"
)

// trashFixture gives a synced account with a trash folder and one message in
// the inbox.
func trashFixture(t *testing.T, trashName string) (*MailService, int64, int64, int64) {
	t.Helper()

	svc, _, db := newTestService(t, stubBackend{})
	ctx := context.Background()

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	if trashName != "" {
		if err := db.UpsertFolders(ctx, acct.ID, []model.Folder{
			{Path: trashName, Name: trashName},
		}); err != nil {
			t.Fatalf("UpsertFolders() error: %v", err)
		}
	}

	folders, _ := svc.ListFolders(acct.ID)
	var inbox, trash int64
	for _, f := range folders {
		switch {
		case f.IsInbox:
			inbox = f.ID
		case f.Name == trashName:
			trash = f.ID
		}
	}

	msgs, err := svc.ListMessages(inbox, 10, 0, false)
	if err != nil || len(msgs) == 0 {
		t.Fatalf("ListMessages() = %v, %v", msgs, err)
	}
	return svc, msgs[0].ID, inbox, trash
}

func messageCount(t *testing.T, svc *MailService, folderID int64) int {
	t.Helper()
	msgs, err := svc.ListMessages(folderID, 100, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	return len(msgs)
}

// Delete used to mean UID EXPUNGE: the message was gone from the server with
// no trash to find it in and no way back. That is not what Delete means in any
// mail client anybody has used, and it is not a mistake a person gets to make
// twice.
func TestDeletingMovesToTheTrash(t *testing.T) {
	svc, id, inbox, trash := trashFixture(t, "Çöp Kutusu")
	if trash == 0 {
		t.Fatal("the fixture has no trash folder")
	}

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}

	if got := messageCount(t, svc, inbox); got != 0 {
		t.Errorf("the inbox still holds %d messages", got)
	}

	ops, err := svc.store.ClaimOperations(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf("ClaimOperations() error: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("queued %d operations, want 1", len(ops))
	}
	if ops[0].Kind != model.OpMove {
		t.Errorf("queued a %s; deleting has to be a move to the trash", ops[0].Kind)
	}
	if ops[0].TargetFolderID != trash {
		t.Errorf("moved to folder %d, want the trash %d", ops[0].TargetFolderID, trash)
	}
}

// A Turkish account's trash is not called Trash. Finding it by role is the
// whole reason roles exist.
func TestTheTrashIsFoundByRoleNotByName(t *testing.T) {
	for _, name := range []string{"Trash", "Çöp Kutusu", "Deleted Items", "Silinmiş Öğeler"} {
		svc, id, _, trash := trashFixture(t, name)
		if trash == 0 {
			t.Fatalf("%q was not recognised as a trash folder", name)
		}

		if err := svc.DeleteMessages([]int64{id}); err != nil {
			t.Fatalf("DeleteMessages() error: %v", err)
		}
		ops, _ := svc.store.ClaimOperations(context.Background(), 1, 10)
		if len(ops) != 1 || ops[0].Kind != model.OpMove {
			t.Errorf("deleting into %q queued %+v, want a move", name, ops)
		}
	}
}

// Otherwise the trash could never be emptied and "delete" there would do
// nothing at all.
func TestDeletingFromTheTrashDestroys(t *testing.T) {
	svc, id, _, trash := trashFixture(t, "Çöp Kutusu")

	// Put the message in the trash the way the move would have.
	if err := svc.MoveMessages([]int64{id}, trash); err != nil {
		t.Fatalf("MoveMessages() error: %v", err)
	}
	// The move deletes the local row; the destination's next sync brings it
	// back. Stand in for that sync.
	ctx := context.Background()
	if err := svc.store.UpsertMessages(ctx, trash, []model.Message{{
		AccountID: 1, FolderID: trash, UID: 500, Subject: "Silinecek",
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}
	inTrash, _ := svc.ListMessages(trash, 10, 0, false)
	if len(inTrash) != 1 {
		t.Fatalf("the trash holds %d messages, want 1", len(inTrash))
	}

	// Clear the move so the next claim sees only what this delete queues.
	claimed, _ := svc.store.ClaimOperations(ctx, 1, 10)
	for _, op := range claimed {
		if err := svc.store.MarkOperationDone(ctx, op.ID); err != nil {
			t.Fatalf("MarkOperationDone() error: %v", err)
		}
	}

	if err := svc.DeleteMessages([]int64{inTrash[0].ID}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}

	ops, _ := svc.store.ClaimOperations(ctx, 1, 10)
	if len(ops) != 1 || ops[0].Kind != model.OpDelete {
		t.Errorf("deleting from the trash queued %+v, want a destroy", ops)
	}
}

// There is nowhere to move the message to, and refusing to delete would be a
// client that cannot delete mail.
func TestWithNoTrashFolderDeletingStillDestroys(t *testing.T) {
	svc, id, _, trash := trashFixture(t, "")
	if trash != 0 {
		t.Fatal("the fixture has a trash folder it should not")
	}

	if err := svc.DeleteMessages([]int64{id}); err != nil {
		t.Fatalf("DeleteMessages() error: %v", err)
	}

	ops, _ := svc.store.ClaimOperations(context.Background(), 1, 10)
	if len(ops) != 1 || ops[0].Kind != model.OpDelete {
		t.Errorf("queued %+v with no trash folder, want a destroy", ops)
	}
}

// A message the retention window removed a moment ago, or one another device
// deleted, is a stale selection rather than an error.
func TestDeletingSomethingThatIsGoneIsNotAnError(t *testing.T) {
	svc, _, _, _ := trashFixture(t, "Çöp Kutusu")

	if err := svc.DeleteMessages([]int64{999999}); err != nil {
		t.Errorf("DeleteMessages() on a missing message: %v", err)
	}
}
