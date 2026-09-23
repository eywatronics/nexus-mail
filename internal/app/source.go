package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// sourcePath serves a message in the form it arrived in.
const sourcePath = "/mail-source/"

// serveSource writes the raw message out as plain text.
//
// Served over HTTP rather than returned through the Wails bridge for the same
// reason bodies are: a large message is a large string, and pushing it through
// an IPC channel to be turned into a DOM node blocks the UI thread for long
// enough to be seen. As text/plain it also cannot be rendered as markup, so
// the source view is the one place a message is guaranteed inert.
func (h *BodyHandler) serveSource(w http.ResponseWriter, r *http.Request) {
	id, err := messageIDFromPath(r.URL.Path, sourcePath)
	if err != nil {
		http.Error(w, "bad message id", http.StatusBadRequest)
		return
	}

	raw, err := h.svc.rawMessage(r.Context(), id)
	if err != nil {
		http.Error(w, "message unavailable", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := w.Write(displayableUTF8(raw)); err != nil {
		return
	}
}

// displayableUTF8 replaces byte sequences that are not valid UTF-8, so the
// declared charset is not a lie.
//
// This matters precisely for the messages the source view is most often opened
// for: one whose body is ISO-8859-9 is not valid UTF-8, and serving it as if it
// were leaves the browser to guess. The replacement character is visible and
// honest — it says "these bytes were not text in this encoding", which is the
// answer the reader came for. The untouched bytes are one click away as an
// .eml file.
func displayableUTF8(raw []byte) []byte {
	if utf8.Valid(raw) {
		return raw
	}

	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); {
		r, size := utf8.DecodeRune(raw[i:])
		if r == utf8.RuneError && size == 1 {
			out = append(out, "�"...)
		} else {
			out = append(out, raw[i:i+size]...)
		}
		i += size
	}
	return out
}

// rawMessage fetches one message in the form it arrived in.
func (s *MailService) rawMessage(ctx context.Context, messageID int64) ([]byte, error) {
	msg, folder, acct, err := s.locateMessage(ctx, messageID)
	if err != nil {
		return nil, err
	}
	return s.engine.FetchRawMessage(ctx, acct, folder, msg)
}

// SaveMessageAsEML writes a message to disk in the form it arrived in.
//
// The file goes to the app's own directory rather than through a save dialog.
// A dialog would tie this service to the window toolkit and make it
// untestable; RevealMessageEML then puts the reader in front of the file with
// their own file manager, which is where they would have picked a destination
// anyway and where they can move it wherever they like.
//
// The .eml is the message byte for byte. That is what makes it openable by any
// other mail client, and what makes it worth anything as a record of what was
// actually received.
func (s *MailService) SaveMessageAsEML(messageID int64) (string, error) {
	ctx := context.Background()

	raw, err := s.rawMessage(ctx, messageID)
	if err != nil {
		return "", err
	}

	dirFn := s.cfg.AttachmentDir
	if dirFn == nil {
		return "", fmt.Errorf("app: no directory configured to save into")
	}
	dir, err := dirFn()
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, emlFilename(messageID, s.subjectOf(ctx, messageID)))
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return "", fmt.Errorf("app: saving the message: %w", err)
	}
	return path, nil
}

// RevealMessageEML saves the message and opens the folder it landed in.
//
// The folder, not the file — the same rule attachments follow. An .eml handed
// to the operating system opens in whatever is registered for it, which on a
// machine with another mail client installed is that client, reading a message
// this one already has.
func (s *MailService) RevealMessageEML(messageID int64) error {
	path, err := s.SaveMessageAsEML(messageID)
	if err != nil {
		return err
	}
	return revealInFileManager(path)
}

// subjectOf reads a message's subject for naming purposes only. An empty
// subject is not an error — plenty of mail has none.
func (s *MailService) subjectOf(ctx context.Context, messageID int64) string {
	var subject string
	row := s.store.Read().QueryRowContext(ctx,
		`SELECT subject FROM messages WHERE id = ?`, messageID)
	if err := row.Scan(&subject); err != nil {
		return ""
	}
	return subject
}

// emlFilename turns a subject into a filename, keeping the message id so two
// mails with the same subject cannot overwrite each other.
//
// The length cap is not cosmetic: a mailing-list subject can run past the
// 255-byte limit most filesystems impose, and the write would fail on a
// message that is otherwise fine. Truncation is on runes, so a Turkish subject
// does not get cut through the middle of a character.
func emlFilename(messageID int64, subject string) string {
	const maxSubjectRunes = 60

	name := safeFilename(strings.TrimSpace(subject))
	if name == "attachment" || name == "" {
		name = "message"
	}
	if runes := []rune(name); len(runes) > maxSubjectRunes {
		name = strings.TrimSpace(string(runes[:maxSubjectRunes]))
	}
	return fmt.Sprintf("%d-%s.eml", messageID, name)
}
