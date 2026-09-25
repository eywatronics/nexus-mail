package app

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"nexusmail/internal/mailmime"
)

// FilePicker asks the person which files to attach.
//
// An interface because the real one needs a window, a desktop and the
// operating system's own dialog, and a test needs none of those. It is also
// what keeps this package free of Wails: the toolkit is imported in main.go
// and nowhere else, which is the property that lets every layer below the
// window be tested without one.
type FilePicker interface {
	// PickFiles returns the chosen paths, or nothing at all when the person
	// closed the dialog. Cancelling is not an error.
	PickFiles(title string) ([]string, error)
}

// maxAttachmentBytes is the most one message may carry, in total.
//
// Not a protocol limit — there is no such thing in SMTP, and the server's own
// SIZE is checked by smtpx before the transaction, where the real number is
// known. This one is about memory: building a message holds the whole thing,
// base64 and all, in this process, and base64 costs a third on top. Forty
// megabytes of files becomes something over fifty in the buffer and roughly
// double that at the moment the message is written out.
//
// It is also comfortably above what almost every server will take, so in
// practice the server's refusal arrives first and says so in its own words.
const maxAttachmentBytes = 40 << 20

// OutgoingAttachmentDTO is one file the person chose, described but not read.
//
// Named for its direction because AttachmentDTO is already taken by the other
// one: a part of a message that arrived, which has an id, a database row and a
// download state. These two have almost nothing in common beyond a filename,
// and one type serving both would be a type where half the fields are always
// empty.
//
// The path travels rather than the bytes. An eight-megabyte PDF serialised
// into an IPC message would block the window for long enough to be seen, and
// the file has to be read at send time anyway.
type OutgoingAttachmentDTO struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	// MIMEType is a guess from the file's extension, and empty when there is
	// nothing to guess from. The message then declares the generic binary
	// type: a wrong type is worse than a vague one, because it tells the
	// reader's client to open the file with the wrong thing.
	MIMEType string `json:"mimeType"`
}

// PickAttachments opens the operating system's file dialog and describes what
// came back.
//
// Every path it returns is remembered, and SendMessage will read no other.
// Without that, this pair of methods would amount to "the window may ask the
// backend to read any file on the machine and mail it somewhere" — which is
// not a hole anything can reach today, but it is the shape of one, and the
// list costs a map.
func (s *MailService) PickAttachments() ([]OutgoingAttachmentDTO, error) {
	if s.picker == nil {
		return nil, fmt.Errorf("app: this build cannot open a file dialog")
	}

	paths, err := s.picker.PickFiles("Attach files")
	if err != nil {
		return nil, fmt.Errorf("app: choosing files: %w", err)
	}

	out := make([]OutgoingAttachmentDTO, 0, len(paths))
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("app: %s cannot be read", filepath.Base(p))
		}
		if info.IsDir() {
			// A folder is not a file, and quietly attaching nothing would be
			// worse than saying so.
			return nil, fmt.Errorf("app: %s is a folder", filepath.Base(p))
		}

		s.allowAttachment(p)
		out = append(out, OutgoingAttachmentDTO{
			Path:     p,
			Name:     filepath.Base(p),
			Size:     info.Size(),
			MIMEType: mimeTypeOf(p),
		})
	}
	return out, nil
}

// SetFilePicker hands the service a way to ask for files. Called once at
// startup by the code that owns the window.
//
// A package function rather than a method, and that is the whole point: Wails
// binds every exported method on a service, so a method here would put "swap
// the file dialog" on the window's own API surface — and the binding generator
// says as much, since an interface cannot cross the bridge at all. Start-up
// wiring is not something the window should be able to do.
func SetFilePicker(s *MailService, p FilePicker) {
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	s.picker = p
}

func (s *MailService) allowAttachment(path string) {
	s.attachMu.Lock()
	defer s.attachMu.Unlock()
	if s.attachable == nil {
		s.attachable = map[string]bool{}
	}
	s.attachable[path] = true
}

func (s *MailService) mayAttach(path string) bool {
	s.attachMu.RLock()
	defer s.attachMu.RUnlock()
	return s.attachable[path]
}

// readAttachments turns the paths the window sent back into content.
//
// Read here, at send time, rather than held in memory from the moment the
// person chose them: a composer left open over lunch with three photographs on
// it would otherwise keep all three in the process the whole time, and what
// gets sent should be the file as it is when Send is pressed.
func (s *MailService) readAttachments(paths []string) ([]mailmime.Attachment, error) {
	if len(paths) == 0 {
		return nil, nil
	}

	// Every check first, then the reading. Measuring as the files came in
	// would mean the one that breaks the cap had already been read into this
	// process to be measured — which is the thing the cap exists to prevent.
	var total int64
	for _, p := range paths {
		if !s.mayAttach(p) {
			// Not "no such file": the file may well exist. This is a path
			// nobody chose in a dialog, and the difference matters to whoever
			// reads the error.
			return nil, fmt.Errorf("app: %s was not chosen in the file dialog",
				filepath.Base(p))
		}

		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("app: %s could not be read", filepath.Base(p))
		}
		total += info.Size()
	}
	if total > maxAttachmentBytes {
		return nil, fmt.Errorf(
			"app: the attachments come to more than %d MB, which is more than "+
				"this message can carry", maxAttachmentBytes>>20)
	}

	out := make([]mailmime.Attachment, 0, len(paths))
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("app: %s could not be read", filepath.Base(p))
		}
		out = append(out, mailmime.Attachment{
			Filename: filepath.Base(p),
			MIMEType: mimeTypeOf(p),
			Content:  content,
		})
	}
	return out, nil
}

// mimeTypeOf guesses from the extension, and gives up rather than guessing
// harder. Sniffing the content would put a type on a file the person chose
// deliberately, and be wrong on exactly the files where it matters.
func mimeTypeOf(path string) string {
	t := mime.TypeByExtension(filepath.Ext(path))
	if t == "" {
		return ""
	}
	// Strip the parameters the table adds, such as "; charset=utf-8". The
	// builder sets the charset it actually used.
	if i := strings.IndexByte(t, ';'); i >= 0 {
		t = strings.TrimSpace(t[:i])
	}
	return t
}
