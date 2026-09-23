package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ListAttachments returns the files a message carries.
//
// Read straight from the database: the parts were recorded when the header was
// synced, so this costs nothing and works with the network off.
func (s *MailService) ListAttachments(messageID int64) ([]AttachmentDTO, error) {
	parts, err := s.store.ListAttachments(context.Background(), messageID)
	if err != nil {
		return nil, err
	}

	out := make([]AttachmentDTO, 0, len(parts))
	for _, p := range parts {
		out = append(out, AttachmentDTO{
			ID:         p.ID,
			Filename:   p.Filename,
			MIMEType:   p.MIMEType,
			Size:       p.Size,
			Downloaded: p.Downloaded,
			LocalPath:  p.LocalPath,
		})
	}
	return out, nil
}

// DownloadAttachment fetches an attachment's bytes and saves them.
//
// A file already on disk is returned as it is. Fetching it again would make
// opening a large attachment twice cost twice, for a file the server has not
// changed — attachments are immutable once a message exists.
func (s *MailService) DownloadAttachment(attachmentID int64) (AttachmentDTO, error) {
	ctx := context.Background()

	part, messageID, err := s.store.GetAttachment(ctx, attachmentID)
	if err != nil {
		return AttachmentDTO{}, err
	}
	if part.Downloaded && part.LocalPath != "" {
		if _, statErr := os.Stat(part.LocalPath); statErr == nil {
			return AttachmentDTO{
				ID: part.ID, Filename: part.Filename, MIMEType: part.MIMEType,
				Size: part.Size, Downloaded: true, LocalPath: part.LocalPath,
			}, nil
		}
		// The record says it is here and it is not — the user cleared the
		// cache, or a sync purge took it. Fetching again is the right answer.
	}

	msg, folder, acct, err := s.locateMessage(ctx, messageID)
	if err != nil {
		return AttachmentDTO{}, err
	}

	data, err := s.engine.FetchAttachment(ctx, acct, folder, msg, part)
	if err != nil {
		return AttachmentDTO{}, err
	}

	path, err := s.saveAttachment(part.ID, part.Filename, data)
	if err != nil {
		return AttachmentDTO{}, err
	}
	if err := s.store.MarkAttachmentDownloaded(ctx, part.ID, path); err != nil {
		return AttachmentDTO{}, err
	}

	return AttachmentDTO{
		ID: part.ID, Filename: part.Filename, MIMEType: part.MIMEType,
		Size: part.Size, Downloaded: true, LocalPath: path,
	}, nil
}

// saveAttachment writes the bytes into the attachment directory.
//
// The file is named after the row id as well as the filename, because two
// messages routinely carry files with the same name and one must not overwrite
// the other.
func (s *MailService) saveAttachment(id int64, filename string, data []byte) (string, error) {
	dirFn := s.cfg.AttachmentDir
	if dirFn == nil {
		return "", fmt.Errorf("app: no attachment directory configured")
	}
	dir, err := dirFn()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, fmt.Sprintf("%d-%s", id, safeFilename(filename)))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("app: saving attachment: %w", err)
	}
	return path, nil
}

// safeFilename reduces a filename from a message to something that can only
// name a file inside the directory it is joined to.
//
// The name comes from whoever sent the mail. Joining it to a path unchanged is
// how a client is talked into writing over something it had no business
// touching: "../../../etc/passwd" is a filename as far as MIME is concerned.
// Separators, parent references and control characters all go.
func safeFilename(name string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r == '/' || r == '\\' || r == ':':
			return '_'
		case unicode.IsControl(r):
			return -1
		default:
			return r
		}
	}, name)

	// After the separators are gone a name of only dots is still a way to
	// refer to a directory rather than a file in it.
	cleaned = strings.Trim(cleaned, ". ")
	if cleaned == "" {
		return "attachment"
	}
	return cleaned
}

// RevealAttachment opens the folder a downloaded attachment sits in.
//
// The containing folder, never the file. Opening the file would hand it to
// whatever the operating system thinks should run it, and an attachment is a
// stranger's file: that is how a mail client becomes the last step of an
// attack. Showing the reader where it landed lets them decide what opens it.
func (s *MailService) RevealAttachment(attachmentID int64) error {
	saved, err := s.DownloadAttachment(attachmentID)
	if err != nil {
		return err
	}
	return revealInFileManager(saved.LocalPath)
}
