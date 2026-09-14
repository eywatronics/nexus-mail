package store

import (
	"context"
	"testing"
	"time"

	"nexusmail/internal/model"
)

func seedMessageWithParts(t *testing.T, s *Store, acct, folderID int64, parts []model.AttachmentPart) int64 {
	t.Helper()
	ctx := context.Background()

	if err := s.UpsertMessages(ctx, folderID, []model.Message{{
		AccountID: acct, FolderID: folderID, UID: 1,
		Subject: "Rapor ektedir", InternalDate: time.Unix(1, 0),
		HasAttachments: len(parts) > 0,
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	msgs, err := s.ListMessages(ctx, folderID, 10, 0)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	id := msgs[0].ID

	if err := s.UpsertAttachments(ctx, id, parts); err != nil {
		t.Fatalf("UpsertAttachments() error: %v", err)
	}
	return id
}

// The paperclip says a message has files; this is what lets the reader see
// which ones without waiting for a download.
func TestAttachmentsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	id := seedMessageWithParts(t, s, acct, inbox, []model.AttachmentPart{
		{PartID: "2", Filename: "rapor.pdf", MIMEType: "application/pdf", Size: 2048, Encoding: "base64"},
		{PartID: "3", Filename: "tablo.xlsx", MIMEType: "application/vnd.ms-excel", Size: 512},
	})

	got, err := s.ListAttachments(ctx, id)
	if err != nil {
		t.Fatalf("ListAttachments() error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("stored %d attachments, want 2", len(got))
	}
	if got[0].Filename != "rapor.pdf" || got[0].Size != 2048 {
		t.Errorf("first attachment = %+v", got[0])
	}
	if got[0].ID == 0 {
		t.Error("ID = 0; nothing can ask for this file")
	}
	if got[0].Downloaded {
		t.Error("a freshly listed attachment claims to be downloaded")
	}
}

// A header resync describes the same parts again. Without an upsert every sync
// would double the list, and a message would grow attachments it never had.
func TestUpsertAttachmentsIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	parts := []model.AttachmentPart{
		{PartID: "2", Filename: "rapor.pdf", MIMEType: "application/pdf", Size: 2048},
	}
	id := seedMessageWithParts(t, s, acct, inbox, parts)

	for range 3 {
		if err := s.UpsertAttachments(ctx, id, parts); err != nil {
			t.Fatalf("UpsertAttachments() error: %v", err)
		}
	}

	got, _ := s.ListAttachments(ctx, id)
	if len(got) != 1 {
		t.Errorf("the message grew to %d attachments after resyncs", len(got))
	}
}

// Re-describing a part must not forget that its bytes are already on disk, or
// every sync would make the reader download the same file again.
func TestUpsertAttachmentsKeepsTheDownloadedFile(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	parts := []model.AttachmentPart{{PartID: "2", Filename: "rapor.pdf", Size: 10}}
	id := seedMessageWithParts(t, s, acct, inbox, parts)

	got, _ := s.ListAttachments(ctx, id)
	if err := s.MarkAttachmentDownloaded(ctx, got[0].ID, "/tmp/rapor.pdf"); err != nil {
		t.Fatalf("MarkAttachmentDownloaded() error: %v", err)
	}

	if err := s.UpsertAttachments(ctx, id, parts); err != nil {
		t.Fatalf("second UpsertAttachments() error: %v", err)
	}

	after, _ := s.ListAttachments(ctx, id)
	if !after[0].Downloaded || after[0].LocalPath == "" {
		t.Errorf("the resync forgot the downloaded file: %+v", after[0])
	}
}

func TestMarkAttachmentDownloadedRecordsThePath(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	id := seedMessageWithParts(t, s, acct, inbox,
		[]model.AttachmentPart{{PartID: "2", Filename: "rapor.pdf", Size: 10}})

	got, _ := s.ListAttachments(ctx, id)
	if err := s.MarkAttachmentDownloaded(ctx, got[0].ID, "/tmp/rapor.pdf"); err != nil {
		t.Fatalf("MarkAttachmentDownloaded() error: %v", err)
	}

	after, _ := s.ListAttachments(ctx, id)
	if !after[0].Downloaded || after[0].LocalPath != "/tmp/rapor.pdf" {
		t.Errorf("attachment = %+v, want it marked downloaded at the path", after[0])
	}
}

// Deleting a message takes its attachment rows with it, or a purge would leave
// orphans pointing at files nothing can reach.
func TestAttachmentsGoWithTheirMessage(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	seedMessageWithParts(t, s, acct, inbox,
		[]model.AttachmentPart{{PartID: "2", Filename: "rapor.pdf", Size: 10}})

	if err := s.DeleteMessagesByUID(ctx, inbox, []uint32{1}); err != nil {
		t.Fatalf("DeleteMessagesByUID() error: %v", err)
	}

	var left int
	if err := s.Read().QueryRowContext(ctx, `SELECT count(*) FROM attachments`).Scan(&left); err != nil {
		t.Fatalf("counting attachments: %v", err)
	}
	if left != 0 {
		t.Errorf("%d attachment rows outlived their message", left)
	}
}

func TestListAttachmentsOnAMessageWithNone(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	id := seedMessageWithParts(t, s, acct, inbox, nil)

	got, err := s.ListAttachments(ctx, id)
	if err != nil {
		t.Fatalf("ListAttachments() error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a message with no files reports %+v", got)
	}
}

// The header sync is the only place that ever sees the BODYSTRUCTURE, so it is
// the only place the attachment list can come from. Writing the message and
// dropping its parts would leave the paperclip with nothing behind it.
func TestUpsertMessagesStoresTheAttachmentsItWasGiven(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	if err := s.UpsertMessages(ctx, inbox, []model.Message{{
		AccountID: acct, FolderID: inbox, UID: 1,
		Subject: "Rapor", InternalDate: time.Unix(1, 0), HasAttachments: true,
		Attachments: []model.AttachmentPart{
			{PartID: "2", Filename: "rapor.pdf", MIMEType: "application/pdf", Size: 900},
		},
	}}); err != nil {
		t.Fatalf("UpsertMessages() error: %v", err)
	}

	msgs, _ := s.ListMessages(ctx, inbox, 10, 0)
	got, err := s.ListAttachments(ctx, msgs[0].ID)
	if err != nil {
		t.Fatalf("ListAttachments() error: %v", err)
	}
	if len(got) != 1 || got[0].Filename != "rapor.pdf" {
		t.Errorf("stored %+v, want the part the sync described", got)
	}
}

// A resync re-describes everything. The attachment list must not grow, and the
// file already on disk must survive.
func TestResyncingAMessageDoesNotDuplicateItsAttachments(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	acct, inbox := seedInbox(t, s)

	msg := model.Message{
		AccountID: acct, FolderID: inbox, UID: 1,
		Subject: "Rapor", InternalDate: time.Unix(1, 0), HasAttachments: true,
		Attachments: []model.AttachmentPart{{PartID: "2", Filename: "rapor.pdf", Size: 900}},
	}
	for range 3 {
		if err := s.UpsertMessages(ctx, inbox, []model.Message{msg}); err != nil {
			t.Fatalf("UpsertMessages() error: %v", err)
		}
	}

	msgs, _ := s.ListMessages(ctx, inbox, 10, 0)
	got, _ := s.ListAttachments(ctx, msgs[0].ID)
	if len(got) != 1 {
		t.Errorf("three syncs produced %d attachment rows, want 1", len(got))
	}
}
