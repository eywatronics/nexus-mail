package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// maxInlineImageBytes keeps one embedded image within a size a mail server will
// actually accept and a WebView can render without a visible pause.
const maxInlineImageBytes = 5 << 20

var contentIDPattern = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,200}$`)

// InlineImages serves the previews the composer shows while an image is still
// a cid: reference in the message.
//
// The map is the whole access control: a path is reachable only after the file
// dialog handed it over (or a draft that did so was reopened), and the URL
// carries a random identifier rather than a filesystem path. A request for an
// identifier nobody registered is a 404, not a file read.
type InlineImages struct {
	mu    sync.RWMutex
	paths map[string]string
}

func NewInlineImages() *InlineImages {
	return &InlineImages{paths: map[string]string{}}
}

// Register makes a path visible at /compose-image/<cid> and returns that URL.
func (h *InlineImages) Register(cid, path string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.paths == nil {
		h.paths = map[string]string{}
	}
	h.paths[cid] = path
	return "/compose-image/" + cid
}

func (h *InlineImages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cid := strings.TrimPrefix(r.URL.Path, "/compose-image/")
	if cid == "" || strings.Contains(cid, "/") {
		http.NotFound(w, r)
		return
	}

	h.mu.RLock()
	path, ok := h.paths[cid]
	h.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}

	content, err := os.ReadFile(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	contentType := mime.TypeByExtension(filepath.Ext(path))
	if !strings.HasPrefix(contentType, "image/") || contentType == "image/svg+xml" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(content); err != nil {
		return
	}
}

// InlineImageDTO is one image the composer will embed.
//
// URL is the preview address; Path is what gets read at send time; CID is what
// the HTML references and what the MIME part's Content-ID carries.
type InlineImageDTO struct {
	Path     string `json:"path"`
	CID      string `json:"cid"`
	Name     string `json:"name"`
	MIMEType string `json:"mimeType"`
	URL      string `json:"url"`
}

// SetInlineImages hands the service the preview registry. Called once at
// startup by the code that owns the HTTP middleware, like SetFilePicker.
func SetInlineImages(s *MailService, images *InlineImages) {
	s.inline = images
}

// PickInlineImage opens the file dialog and returns one image ready to embed.
//
// Cancelling is not an error: the caller gets an empty DTO and simply inserts
// nothing. The file is admitted to the attachment allowlist here, so sending
// can read it later without opening a "mail any path" hole.
func (s *MailService) PickInlineImage() (InlineImageDTO, error) {
	if s.picker == nil {
		return InlineImageDTO{}, fmt.Errorf("app: this build cannot open a file dialog")
	}
	if s.inline == nil {
		return InlineImageDTO{}, fmt.Errorf("app: this build has nowhere to serve image previews")
	}

	paths, err := s.picker.PickFiles("Insert image")
	if err != nil {
		return InlineImageDTO{}, fmt.Errorf("app: choosing an image: %w", err)
	}
	if len(paths) == 0 {
		return InlineImageDTO{}, nil
	}

	path := paths[0]
	info, err := os.Stat(path)
	if err != nil {
		return InlineImageDTO{}, fmt.Errorf("app: %s cannot be read", filepath.Base(path))
	}
	if info.IsDir() {
		return InlineImageDTO{}, fmt.Errorf("app: %s is a folder", filepath.Base(path))
	}
	if info.Size() > maxInlineImageBytes {
		return InlineImageDTO{}, fmt.Errorf(
			"app: %s is larger than the %d MB an embedded image may be",
			filepath.Base(path), maxInlineImageBytes>>20)
	}

	mimeType := mimeTypeOf(path)
	if !strings.HasPrefix(mimeType, "image/") {
		return InlineImageDTO{}, fmt.Errorf("app: %s is not an image", filepath.Base(path))
	}
	if mimeType == "image/svg+xml" {
		// SVG is a document with scripts in it, and not every mail reader
		// renders it. Refusing it is clearer than sending a picture that may
		// arrive as a broken box.
		return InlineImageDTO{}, fmt.Errorf("app: SVG cannot be embedded; use PNG, JPEG or GIF")
	}

	cid, err := newContentID()
	if err != nil {
		return InlineImageDTO{}, err
	}
	s.allowAttachment(path)
	url := s.inline.Register(cid, path)

	return InlineImageDTO{
		Path:     path,
		CID:      cid,
		Name:     filepath.Base(path),
		MIMEType: mimeType,
		URL:      url,
	}, nil
}

func newContentID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("app: generating an image id: %w", err)
	}
	return "nx-" + hex.EncodeToString(buf[:]) + "@nexus-mail", nil
}
