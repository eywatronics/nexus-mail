package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"nexusmail/internal/model"
)

// DraftRecordDTO is a saved draft, on its way to or from the composer.
//
// Separate from DraftDTO, which is a message being sent. They carry nearly the
// same fields and mean different things: one is an instruction, the other is a
// record, and the record has an id and a time on it while the instruction has
// neither. Folding them together would put an id on a send.
type DraftRecordDTO struct {
	// ID is zero for a draft that has never been saved. The composer sends
	// back whatever it was given, which is what makes autosave one row rather
	// than one row per pause.
	ID         int64 `json:"id"`
	AccountID  int64 `json:"accountId"`
	IdentityID int64 `json:"identityId"`

	To      string `json:"to"`
	Cc      string `json:"cc"`
	Bcc     string `json:"bcc"`
	Subject string `json:"subject"`
	Body    string `json:"body"`

	InReplyTo  string   `json:"inReplyTo"`
	References []string `json:"references"`

	// AttachmentPaths is what a save carries: the files by path.
	AttachmentPaths []string `json:"attachmentPaths"`

	// Attachments is what a read carries: the same files, described, so the
	// composer can show their names and sizes without reading them. Empty on
	// the way in.
	Attachments []OutgoingAttachmentDTO `json:"attachments"`
	// MissingAttachments names the files a reopened draft referred to that are
	// no longer on disk. Reported rather than dropped in silence: a draft that
	// quietly comes back with one fewer attachment is a message sent without
	// the thing it was about.
	MissingAttachments []string `json:"missingAttachments"`

	// UpdatedAtUnix is seconds since the epoch, for the same reason message
	// dates are: an explicit integer has no timezone to lose across the bridge.
	UpdatedAtUnix int64 `json:"updatedAtUnix"`
}

// SaveDraft writes what is in the composer and returns the draft's id.
//
// Called on a pause in typing rather than on every keystroke. The window
// decides when; this only has to be cheap enough that it can.
func (s *MailService) SaveDraft(d DraftRecordDTO) (int64, error) {
	if d.AccountID == 0 {
		return 0, fmt.Errorf("app: a draft needs an account it belongs to")
	}
	return s.store.SaveDraft(context.Background(), model.Draft{
		ID:              d.ID,
		AccountID:       d.AccountID,
		IdentityID:      d.IdentityID,
		To:              d.To,
		Cc:              d.Cc,
		Bcc:             d.Bcc,
		Subject:         d.Subject,
		Body:            d.Body,
		InReplyTo:       d.InReplyTo,
		References:      d.References,
		AttachmentPaths: d.AttachmentPaths,
	})
}

// Drafts lists what an account has left unfinished, newest first.
func (s *MailService) Drafts(accountID int64) ([]DraftRecordDTO, error) {
	list, err := s.store.ListDrafts(context.Background(), accountID)
	if err != nil {
		return nil, err
	}

	out := make([]DraftRecordDTO, 0, len(list))
	for _, d := range list {
		out = append(out, draftToDTO(d))
	}
	return out, nil
}

// Draft reads one back, for reopening it.
//
// The attachments are described here and their paths are admitted to the set
// SendMessage will read from. That is not a hole in the allowlist, it is the
// point of it: these paths came out of this application's own database, having
// been chosen in the file dialog when the draft was written. What the list
// keeps out is a path the window made up, and a draft row is not the window.
func (s *MailService) Draft(id int64) (DraftRecordDTO, error) {
	d, err := s.store.GetDraft(context.Background(), id)
	if err != nil {
		return DraftRecordDTO{}, err
	}

	dto := draftToDTO(d)
	for _, p := range d.AttachmentPaths {
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			// A file moved or deleted since the draft was written. The draft
			// still opens — the words in it are worth more than the
			// attachment — but the composer is told what is gone.
			dto.MissingAttachments = append(dto.MissingAttachments, filepath.Base(p))
			continue
		}
		s.allowAttachment(p)
		dto.Attachments = append(dto.Attachments, OutgoingAttachmentDTO{
			Path:     p,
			Name:     filepath.Base(p),
			Size:     info.Size(),
			MIMEType: mimeTypeOf(p),
		})
	}
	return dto, nil
}

// DiscardDraft throws one away, which is what the composer's close button does
// when the person says they meant it.
func (s *MailService) DiscardDraft(id int64) error {
	return s.store.DeleteDraft(context.Background(), id)
}

func draftToDTO(d model.Draft) DraftRecordDTO {
	return DraftRecordDTO{
		ID:              d.ID,
		AccountID:       d.AccountID,
		IdentityID:      d.IdentityID,
		To:              d.To,
		Cc:              d.Cc,
		Bcc:             d.Bcc,
		Subject:         d.Subject,
		Body:            d.Body,
		InReplyTo:       d.InReplyTo,
		References:      d.References,
		AttachmentPaths: d.AttachmentPaths,
		UpdatedAtUnix:   d.UpdatedAt.Unix(),
	}
}
