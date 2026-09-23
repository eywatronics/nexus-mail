package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"nexusmail/internal/imapx"
)

// bodyBackend serves one message whose HTML the test chooses, reusing
// stubBackend for everything else.
type bodyBackend struct {
	stubBackend
	html string
}

func (b bodyBackend) FetchBody(context.Context, uint32) (imapx.Body, error) {
	return imapx.Body{HTML: b.html}, nil
}

func newBodyHandler(t *testing.T, messageHTML string) (*BodyHandler, int64) {
	t.Helper()
	h, id, _ := newBodyHandlerWithEvents(t, messageHTML)
	return h, id
}

// newBodyHandlerWithEvents is the same fixture, keeping the event recorder for
// the tests that care what the handler told the window.
func newBodyHandlerWithEvents(t *testing.T, messageHTML string) (*BodyHandler, int64, *recorder) {
	t.Helper()

	svc, rec, _ := newTestService(t, bodyBackend{html: messageHTML})

	acct, err := svc.AddPasswordAccount("u@example.com", "U", "h", 993, "tls", "", 0, "pw")
	if err != nil {
		t.Fatalf("AddPasswordAccount() error: %v", err)
	}
	if err := svc.SyncAccount(acct.ID); err != nil {
		t.Fatalf("SyncAccount() error: %v", err)
	}

	folders, err := svc.ListFolders(acct.ID)
	if err != nil {
		t.Fatalf("ListFolders() error: %v", err)
	}
	var inboxID int64
	for _, f := range folders {
		if f.IsInbox {
			inboxID = f.ID
		}
	}
	msgs, err := svc.ListMessages(inboxID, 10, 0, false)
	if err != nil {
		t.Fatalf("ListMessages() error: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("no message was synced")
	}

	return NewBodyHandler(svc), msgs[0].ID, rec
}

func get(t *testing.T, h *BodyHandler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestBodyHandlerServesSanitisedHTMLWithARealCSP(t *testing.T) {
	h, id := newBodyHandler(t,
		`<p>Hello</p><script>window.__xssFired = true</script><img src="http://tracker.example/p.png">`)

	rec := get(t, h, fmt.Sprintf("%s%d", bodyPath, id))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	// A header rather than a meta tag: some directives are ignored in meta
	// form, and a header cannot be displaced by markup the message controls.
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'none'") {
		t.Errorf("Content-Security-Policy = %q, want default-src 'none'", csp)
	}
	if rec.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Errorf("Referrer-Policy = %q, want no-referrer", rec.Header().Get("Referrer-Policy"))
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Hello") {
		t.Errorf("body %q lost the message content", body)
	}
	if strings.Contains(body, "__xssFired") {
		t.Errorf("body %q still carries the script", body)
	}
	if strings.Contains(body, "tracker.example/p.png") &&
		!strings.Contains(body, "data-nexus-blocked") {
		t.Errorf("body %q still points at the tracker from a fetching position", body)
	}
}

func TestBodyHandlerRejectsMalformedPaths(t *testing.T) {
	h, _ := newBodyHandler(t, `<p>x</p>`)

	cases := map[string]int{
		bodyPath:                   http.StatusBadRequest,
		bodyPath + "notanumber":    http.StatusBadRequest,
		bodyPath + "../../secrets": http.StatusBadRequest,
		bodyPath + "1/2":           http.StatusBadRequest,
		assetPath + "1":            http.StatusBadRequest,
		"/something-else":          http.StatusNotFound,
	}
	for path, want := range cases {
		if got := get(t, h, path).Code; got != want {
			t.Errorf("GET %q: status = %d, want %d", path, got, want)
		}
	}
}

func TestBodyHandlerReportsAMissingMessage(t *testing.T) {
	h, _ := newBodyHandler(t, `<p>x</p>`)

	// Must be a clean 404 rather than a panic: the message could have been
	// expunged between the list rendering and the click.
	if got := get(t, h, fmt.Sprintf("%s%d", bodyPath, 999999)).Code; got != http.StatusNotFound {
		t.Errorf("status = %d, want 404", got)
	}
}

// Holding the arrow keys changes the selection dozens of times a second.
// Finishing work nobody will see is how an idle client burns a core.
func TestBodyHandlerStopsOnACancelledRequest(t *testing.T) {
	h, id := newBodyHandler(t, `<p>x</p>`)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s%d", bodyPath, id), nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Body.Len() != 0 {
		t.Errorf("a cancelled request still produced %d bytes", rec.Body.Len())
	}
}

func TestBodyHandlerCachesTheRender(t *testing.T) {
	h, id := newBodyHandler(t, `<p>Hello</p>`)
	path := fmt.Sprintf("%s%d", bodyPath, id)

	first := get(t, h, path).Body.String()
	second := get(t, h, path).Body.String()

	if first != second {
		t.Error("two identical requests produced different documents")
	}
	h.mu.Lock()
	cached := len(h.cache)
	h.mu.Unlock()
	if cached != 1 {
		t.Errorf("cache holds %d entries after two identical requests, want 1", cached)
	}
}

// A block-mode render has no token map, so it cannot answer a proxy-mode
// request. Serving it anyway would show the user a message with the images
// still missing after they asked for them.
func TestBodyHandlerDoesNotServeABlockedRenderForAProxyRequest(t *testing.T) {
	h, id := newBodyHandler(t, `<img src="http://images.example/a.png">`)

	get(t, h, fmt.Sprintf("%s%d", bodyPath, id))
	proxied := get(t, h, fmt.Sprintf("%s%d?remote=1", bodyPath, id)).Body.String()

	if !strings.Contains(proxied, "mail-asset") {
		t.Errorf("the consented render %q does not route through the proxy", proxied)
	}
}

func TestAssetProxyServesOnlyTokensFromThisSession(t *testing.T) {
	h, id := newBodyHandler(t, `<img src="http://images.example/a.png">`)

	// Never rendered in proxy mode, so no token exists yet.
	if got := get(t, h, fmt.Sprintf("%s%d/deadbeef", assetPath, id)).Code; got != http.StatusNotFound {
		t.Errorf("status = %d for an unknown token, want 404", got)
	}

	// Render in proxy mode, then ask for a token that still does not exist.
	get(t, h, fmt.Sprintf("%s%d?remote=1", bodyPath, id))
	if got := get(t, h, fmt.Sprintf("%s%d/0000000000000000", assetPath, id)).Code; got != http.StatusNotFound {
		t.Errorf("status = %d for a forged token, want 404", got)
	}
}

// A tokenised asset is still refused when it resolves to a private address.
// Without this a mail image could probe the reader's own network.
func TestAssetProxyRefusesAPrivateOriginEvenWithAValidToken(t *testing.T) {
	var hits atomic.Int64

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		if _, err := w.Write([]byte("fake-png-bytes")); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	defer origin.Close()

	h, id := newBodyHandler(t, fmt.Sprintf(`<img src="%s/pixel.png">`, origin.URL))

	rendered := get(t, h, fmt.Sprintf("%s%d?remote=1", bodyPath, id)).Body.String()
	token := extractToken(t, rendered)

	rec := get(t, h, fmt.Sprintf("%s%d/%s", assetPath, id, token))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if hits.Load() != 0 {
		t.Error("the proxy reached a loopback origin")
	}
}

// The outgoing request must carry nothing that identifies the reader. Referer
// matters most: it would tell the sender which message was opened, which is
// exactly what a tracking pixel is after.
func TestAssetRequestCarriesNoIdentifyingHeaders(t *testing.T) {
	req, err := buildAssetRequest(context.Background(), "https://images.example/pixel.png")
	if err != nil {
		t.Fatalf("buildAssetRequest() error: %v", err)
	}

	for _, header := range []string{"Referer", "Cookie", "Authorization", "From"} {
		if got := req.Header.Get(header); got != "" {
			t.Errorf("%s = %q, want it absent", header, got)
		}
	}
	if ua := req.Header.Get("User-Agent"); strings.Contains(strings.ToLower(ua), "nexus") {
		t.Errorf("User-Agent = %q, which names this client to the sender", ua)
	}
}

func TestAssetProxyRefusesNonImageContent(t *testing.T) {
	// Serving whatever the origin returns would let a sender smuggle content
	// past the content security policy.
	if _, _, err := fetchAsset(context.Background(), "ftp://example.com/x.png"); err == nil {
		t.Error("fetchAsset() accepted a non-HTTP scheme")
	}
}

func TestRefusePrivateAddress(t *testing.T) {
	blocked := []string{
		"127.0.0.1:80", "192.168.1.1:80", "10.0.0.5:443",
		"169.254.169.254:80", // cloud metadata, the classic SSRF target
		"0.0.0.0:80",
	}
	for _, addr := range blocked {
		if err := refusePrivateAddress("tcp", addr, nil); err == nil {
			t.Errorf("refusePrivateAddress(%q) allowed the connection", addr)
		}
	}

	if err := refusePrivateAddress("tcp", "93.184.216.34:443", nil); err != nil {
		t.Errorf("refusePrivateAddress() blocked a public address: %v", err)
	}
}

func extractToken(t *testing.T, html string) string {
	t.Helper()
	const marker = "mail-asset/"
	i := strings.Index(html, marker)
	if i < 0 {
		t.Fatalf("no proxy URL in %q", html)
	}
	rest := html[i+len(marker):]
	slash := strings.Index(rest, "/")
	if slash < 0 {
		t.Fatalf("malformed proxy URL in %q", html)
	}
	rest = rest[slash+1:]
	end := strings.IndexAny(rest, `"' >`)
	if end < 0 {
		t.Fatalf("malformed proxy URL in %q", html)
	}
	return rest[:end]
}

// The three views are three different documents. A cache that answered one
// request with another's render would make the menu look broken.
func TestBodyHandlerServesEachViewSeparately(t *testing.T) {
	h, id := newBodyHandler(t,
		`<p style="color:red" class="hero">Merhaba</p>`)

	rich := get(t, h, fmt.Sprintf("%s%d", bodyPath, id)).Body.String()
	simple := get(t, h, fmt.Sprintf("%s%d?view=simple", bodyPath, id)).Body.String()
	text := get(t, h, fmt.Sprintf("%s%d?view=text", bodyPath, id)).Body.String()

	if !strings.Contains(rich, "style=") {
		t.Errorf("the default view dropped the styling:\n%s", rich)
	}
	if strings.Contains(simple, "style=") || strings.Contains(simple, "class=") {
		t.Errorf("the simple view kept the decoration:\n%s", simple)
	}
	// "<p" would also match the "<pre>" the text view wraps its output in,
	// which is exactly the tag that proves it worked.
	if strings.Contains(text, "<p>") || strings.Contains(text, "<p ") {
		t.Errorf("the text view still has markup:\n%s", text)
	}
	if !strings.Contains(text, "<pre>") {
		t.Errorf("the text view is not wrapped as preformatted text:\n%s", text)
	}
	for _, doc := range []string{rich, simple, text} {
		if !strings.Contains(doc, "Merhaba") {
			t.Errorf("a view lost the message:\n%s", doc)
		}
	}
}

// The parameter arrives in a URL, and the answer to a malformed one is the
// default view, not a blank reading pane.
func TestAnUnknownViewFallsBackToTheDefault(t *testing.T) {
	h, id := newBodyHandler(t, `<p style="color:red">Merhaba</p>`)

	rec := get(t, h, fmt.Sprintf("%s%d?view=nonsense", bodyPath, id))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d for an unknown view, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "style=") {
		t.Errorf("an unknown view did not fall back to the rich render:\n%s", rec.Body.String())
	}
}

// Plenty of mail is HTML only, and on exactly that mail the reader is most
// likely to want the decoration gone. Handing back the HTML would be answering
// a different question.
func TestTheTextViewReducesAMessageWithNoPlainPart(t *testing.T) {
	h, id := newBodyHandler(t, `<div><h1>Başlık</h1><p>Gövde</p></div>`)

	body := get(t, h, fmt.Sprintf("%s%d?view=text", bodyPath, id)).Body.String()

	if strings.Contains(body, "<h1") || strings.Contains(body, "<div") {
		t.Errorf("the text view served markup:\n%s", body)
	}
	if !strings.Contains(body, "Başlık") || !strings.Contains(body, "Gövde") {
		t.Errorf("the text view lost the words:\n%s", body)
	}
}

// A reader who chose dark on a light machine used to get a dark window with a
// white reading pane in the middle of it — the one place the mismatch is
// impossible to miss. The sandboxed frame cannot see the class on the host
// page, so the window has to say which side it is on.
func TestTheFrameFollowsTheAppsThemeNotTheMachines(t *testing.T) {
	h, id := newBodyHandler(t, `<p>Merhaba</p>`)

	dark := get(t, h, fmt.Sprintf("%s%d?theme=dark", bodyPath, id)).Body.String()
	light := get(t, h, fmt.Sprintf("%s%d?theme=light", bodyPath, id)).Body.String()

	if !strings.Contains(dark, "#0a0a0a") {
		t.Errorf("the dark frame does not use the dark background:\n%s", dark)
	}
	if strings.Contains(dark, "prefers-color-scheme") {
		t.Errorf("an explicit choice still defers to the machine:\n%s", dark)
	}
	if !strings.Contains(light, "#fff") || strings.Contains(light, "#0a0a0a") {
		t.Errorf("the light frame is not light:\n%s", light)
	}
}

// Without color-scheme the frame's scrollbar and any form control the message
// carries stay light on a dark page.
func TestTheDarkFrameDeclaresItsColorScheme(t *testing.T) {
	h, id := newBodyHandler(t, `<p>Merhaba</p>`)

	dark := get(t, h, fmt.Sprintf("%s%d?theme=dark", bodyPath, id)).Body.String()
	if !strings.Contains(dark, "color-scheme:dark") {
		t.Errorf("the dark frame does not declare color-scheme:\n%s", dark)
	}
}

// With no theme named the frame follows the machine, which is what the app
// itself does until the reader picks a side.
func TestWithNoThemeTheFrameFollowsTheMachine(t *testing.T) {
	h, id := newBodyHandler(t, `<p>Merhaba</p>`)

	for _, path := range []string{
		fmt.Sprintf("%s%d", bodyPath, id),
		fmt.Sprintf("%s%d?theme=nonsense", bodyPath, id),
	} {
		body := get(t, h, path).Body.String()
		if !strings.Contains(body, "prefers-color-scheme:dark") {
			t.Errorf("%s does not follow the machine:\n%s", path, body)
		}
		if !strings.Contains(body, "color-scheme:light dark") {
			t.Errorf("%s does not declare both schemes:\n%s", path, body)
		}
	}
}

// The theme changes the frame, not the message. Keying the render cache on it
// would mean re-sanitising every message the first time somebody flips the
// switch, for a difference of one style block.
func TestSwitchingThemeDoesNotReRenderTheMessage(t *testing.T) {
	h, id := newBodyHandler(t, `<p>Merhaba</p>`)

	get(t, h, fmt.Sprintf("%s%d?theme=light", bodyPath, id))
	get(t, h, fmt.Sprintf("%s%d?theme=dark", bodyPath, id))

	h.mu.Lock()
	cached := len(h.cache)
	h.mu.Unlock()
	if cached != 1 {
		t.Errorf("cache holds %d entries after two themes of one message, want 1", cached)
	}
}
