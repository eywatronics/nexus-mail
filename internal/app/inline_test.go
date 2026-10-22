package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type imagePicker struct {
	paths []string
	err   error
}

func (p imagePicker) PickFiles(string) ([]string, error) { return p.paths, p.err }

func writeTempImage(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
	return path
}

func TestInlineImagesServesOnlyRegisteredFiles(t *testing.T) {
	path := writeTempImage(t, "grafik.png", []byte("png-bytes"))
	images := NewInlineImages()
	url := images.Register("nx-1@nexus-mail", path)
	if url != "/compose-image/nx-1@nexus-mail" {
		t.Fatalf("Register() = %q", url)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	images.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", url, rec.Code)
	}
	if rec.Body.String() != "png-bytes" {
		t.Errorf("body = %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}

	// The identifier is the access control: a path nobody registered is a
	// 404, not a file read.
	rec = httptest.NewRecorder()
	images.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/compose-image/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET an unregistered image = %d, want 404", rec.Code)
	}

	rec = httptest.NewRecorder()
	images.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, url, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST an image = %d, want 405", rec.Code)
	}
}

func TestPickInlineImageRegistersAPreview(t *testing.T) {
	path := writeTempImage(t, "grafik.png", []byte("png-bytes"))
	svc, _, _ := newTestService(t, stubBackend{})
	svc.inline = NewInlineImages()
	svc.picker = imagePicker{paths: []string{path}}

	got, err := svc.PickInlineImage()
	if err != nil {
		t.Fatalf("PickInlineImage() error: %v", err)
	}
	if got.CID == "" || got.URL != "/compose-image/"+got.CID {
		t.Errorf("got cid %q url %q", got.CID, got.URL)
	}
	if got.Name != "grafik.png" || got.MIMEType != "image/png" {
		t.Errorf("got %+v", got)
	}

	rec := httptest.NewRecorder()
	svc.inline.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, got.URL, nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "png-bytes" {
		t.Errorf("preview = %d %q", rec.Code, rec.Body.String())
	}
}

func TestPickInlineImageRefusesWhatItCannotEmbed(t *testing.T) {
	svc, _, _ := newTestService(t, stubBackend{})
	svc.inline = NewInlineImages()

	svc.picker = imagePicker{paths: []string{writeTempImage(t, "not.txt", []byte("x"))}}
	if _, err := svc.PickInlineImage(); err == nil {
		t.Error("PickInlineImage() accepted a text file")
	}

	svc.picker = imagePicker{paths: []string{writeTempImage(t, "vector.svg", []byte("<svg/>"))}}
	if _, err := svc.PickInlineImage(); err == nil {
		t.Error("PickInlineImage() accepted SVG")
	}

	svc.picker = imagePicker{paths: nil}
	got, err := svc.PickInlineImage()
	if err != nil {
		t.Fatalf("cancelling returned an error: %v", err)
	}
	if got.CID != "" {
		t.Error("cancelling returned an image")
	}
}

func TestSendMessageCarriesInlineImages(t *testing.T) {
	svc, account := sendFixture(t)
	svc.inline = NewInlineImages()
	path := writeTempImage(t, "grafik.png", []byte("png-bytes"))
	svc.picker = imagePicker{paths: []string{path}}

	image, err := svc.PickInlineImage()
	if err != nil {
		t.Fatalf("PickInlineImage() error: %v", err)
	}

	if _, err := svc.SendMessage(DraftDTO{
		AccountID:    account,
		To:           "r@example.com",
		Subject:      "Konu",
		Text:         "metin",
		HTML:         `<p><img src="cid:` + image.CID + `"></p>`,
		InlineImages: []InlineImageDTO{image},
	}); err != nil {
		t.Fatalf("SendMessage() error: %v", err)
	}

	_, raw := queued(t, svc, account)
	for _, want := range []string{
		"multipart/related",
		"Content-Id: <" + image.CID + ">",
		"cid:" + image.CID,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the queued message does not contain %q:\n%s", want, raw)
		}
	}
}

func TestInlineImagesRequireAnAllowedPath(t *testing.T) {
	svc, account := sendFixture(t)
	svc.inline = NewInlineImages()
	path := writeTempImage(t, "grafik.png", []byte("png-bytes"))

	_, err := svc.SendMessage(DraftDTO{
		AccountID: account,
		To:        "r@example.com",
		Text:      "metin",
		HTML:      `<p><img src="cid:nx-1@nexus-mail"></p>`,
		InlineImages: []InlineImageDTO{{
			Path: path, CID: "nx-1@nexus-mail", Name: "grafik.png", MIMEType: "image/png",
		}},
	})
	if err == nil {
		t.Error("SendMessage() read a path that was never chosen in the dialog")
	}
}

func TestInlineImageIdentifiersAreValidated(t *testing.T) {
	svc, account := sendFixture(t)
	svc.inline = NewInlineImages()
	path := writeTempImage(t, "grafik.png", []byte("png-bytes"))
	svc.allowAttachment(path)

	for _, cid := range []string{"", "bad id", "nx-1\r\nX-Evil: yes"} {
		_, err := svc.SendMessage(DraftDTO{
			AccountID: account, To: "r@example.com", Text: "metin",
			HTML:         `<p><img src="cid:` + cid + `"></p>`,
			InlineImages: []InlineImageDTO{{Path: path, CID: cid, Name: "x.png"}},
		})
		if err == nil {
			t.Errorf("SendMessage() accepted content id %q", cid)
		}
	}
}

func TestInlineImagePreviewServesNothingForAMissingFile(t *testing.T) {
	images := NewInlineImages()
	url := images.Register("nx-1@nexus-mail", filepath.Join(t.TempDir(), "gone.png"))
	rec := httptest.NewRecorder()
	images.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing file served as %d, want 404", rec.Code)
	}
}
