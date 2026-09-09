package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"nexusmail/internal/mailhtml"
	"nexusmail/internal/model"
)

// Paths served under the app's own scheme.
const (
	bodyPath  = "/mail-body/"
	assetPath = "/mail-asset/"
)

// contentSecurityPolicy is delivered as a real header rather than a meta tag.
// A header is stronger: some directives are ignored in meta form, and it
// cannot be displaced by markup the message controls.
//
// default-src 'none' means nothing loads unless a later directive allows it.
// Images are limited to data:, cid: and our own scheme — never the network
// directly, so even a bug in the rewriting cannot turn into a request to the
// sender.
const contentSecurityPolicy = "default-src 'none'; " +
	"img-src data: cid: wails:; " +
	"style-src 'unsafe-inline'; " +
	"font-src data:; " +
	"form-action 'none'; " +
	"base-uri 'none'"

// Limits on what the proxy will fetch on the user's behalf.
const (
	maxAssetBytes   = 8 << 20 // 8 MiB
	assetFetchLimit = 15 * time.Second
	renderCacheSize = 32
)

// BodyHandler serves message bodies and proxied images over the app's own
// scheme.
//
// Bodies do not cross the Wails bridge as JSON. An eight-megabyte newsletter
// serialised into an IPC message would block the UI thread for long enough to
// be seen, and srcdoc cannot carry a real CSP header.
type BodyHandler struct {
	svc *MailService

	mu    sync.Mutex
	cache map[int64]*renderedBody
	order []int64
}

// renderedBody is one sanitised message, plus the proxy tokens its HTML refers
// to. Keeping the token map means a proxy request needs no re-sanitising.
type renderedBody struct {
	html   string
	remote map[string]string
	mode   mailhtml.Mode
}

func NewBodyHandler(svc *MailService) *BodyHandler {
	return &BodyHandler{svc: svc, cache: map[int64]*renderedBody{}}
}

func (h *BodyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, bodyPath):
		h.serveBody(w, r)
	case strings.HasPrefix(r.URL.Path, assetPath):
		h.serveAsset(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *BodyHandler) serveBody(w http.ResponseWriter, r *http.Request) {
	id, err := messageIDFromPath(r.URL.Path, bodyPath)
	if err != nil {
		http.Error(w, "bad message id", http.StatusBadRequest)
		return
	}

	mode := mailhtml.ModeBlock
	if r.URL.Query().Get("remote") == "1" {
		mode = mailhtml.ModeProxy
	}

	rendered, err := h.render(r.Context(), id, mode)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// The user moved on before this finished. Holding the arrow keys
			// changes the selection dozens of times a second, and finishing
			// work nobody will see is how an idle client burns a core.
			return
		}
		http.Error(w, "message unavailable", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := io.WriteString(w, wrapDocument(rendered.html)); err != nil {
		return
	}
}

// render produces the sanitised body for a message, fetching and caching it if
// necessary. Cancellation is honoured before each expensive step.
func (h *BodyHandler) render(ctx context.Context, id int64, mode mailhtml.Mode) (*renderedBody, error) {
	if cached := h.lookup(id, mode); cached != nil {
		return cached, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	msg, folder, acct, err := h.svc.locateMessage(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	body, err := h.svc.engine.EnsureBody(ctx, acct, folder, msg)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	raw := body.HTML
	if raw == "" {
		raw = "<pre>" + htmlEscape(body.Text) + "</pre>"
	}

	res, err := mailhtml.Sanitize(raw, mailhtml.Options{
		Mode: mode,
		ProxyURL: func(token string) string {
			return fmt.Sprintf("wails://nexus%s%d/%s", assetPath, id, token)
		},
	})
	if err != nil {
		return nil, err
	}

	rendered := &renderedBody{html: res.HTML, remote: res.RemoteURLs, mode: mode}
	h.store(id, rendered)
	return rendered, nil
}

func (h *BodyHandler) lookup(id int64, mode mailhtml.Mode) *renderedBody {
	h.mu.Lock()
	defer h.mu.Unlock()

	// A cached block-mode render cannot answer a proxy-mode request: it has no
	// token map and no image references at all.
	if got, ok := h.cache[id]; ok && got.mode == mode {
		return got
	}
	return nil
}

func (h *BodyHandler) store(id int64, rendered *renderedBody) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.cache[id]; !exists {
		h.order = append(h.order, id)
	}
	h.cache[id] = rendered

	for len(h.order) > renderCacheSize {
		oldest := h.order[0]
		h.order = h.order[1:]
		delete(h.cache, oldest)
	}
}

// serveAsset fetches one remote image on the reader's behalf.
//
// This is what makes "load remote content" safe to offer at all: the request
// leaves from here, carrying no Referer, no cookies and no user agent that
// identifies the reader, so the sender learns nothing beyond the fact that
// somebody, somewhere, looked.
func (h *BodyHandler) serveAsset(w http.ResponseWriter, r *http.Request) {
	id, token, err := assetRefFromPath(r.URL.Path)
	if err != nil {
		http.Error(w, "bad asset reference", http.StatusBadRequest)
		return
	}

	// The token must come from a render this session produced. An arbitrary
	// URL supplied by the page would turn the proxy into an open relay.
	rendered := h.lookup(id, mailhtml.ModeProxy)
	if rendered == nil {
		http.Error(w, "unknown asset", http.StatusNotFound)
		return
	}
	original, ok := rendered.remote[token]
	if !ok {
		http.Error(w, "unknown asset", http.StatusNotFound)
		return
	}

	data, contentType, err := fetchAsset(r.Context(), original)
	if err != nil {
		http.Error(w, "asset unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(data); err != nil {
		return
	}
}

// assetClient refuses redirects and private addresses.
//
// Following a redirect blindly would let a sender bounce the proxy at a
// machine on the reader's own network, turning a mail image into a port scan.
var assetClient = &http.Client{
	Timeout: assetFetchLimit,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("app: the asset proxy does not follow redirects")
	},
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 10 * time.Second,
			Control: refusePrivateAddress,
		}).DialContext,
		DisableKeepAlives: true,
	},
}

// buildAssetRequest constructs the outgoing request, carrying nothing that
// identifies the reader.
//
// The absence of Referer matters most: it would tell the sender which message
// was opened, which is precisely what a tracking pixel is after. The user
// agent is a generic string rather than one naming this client and version.
func buildAssetRequest(ctx context.Context, rawURL string) (*http.Request, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("app: refusing to proxy scheme %q", parsed.Scheme)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "image/*")
	return req, nil
}

func fetchAsset(ctx context.Context, rawURL string) ([]byte, string, error) {

	req, err := buildAssetRequest(ctx, rawURL)
	if err != nil {
		return nil, "", err
	}

	resp, err := assetClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("app: asset returned status %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		// The renderer asked for an image; serving anything else would let a
		// sender smuggle content past the CSP.
		return nil, "", fmt.Errorf("app: refusing to proxy content type %q", contentType)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes))
	if err != nil {
		return nil, "", err
	}
	return data, contentType, nil
}

// refusePrivateAddress blocks connections to loopback, link-local and private
// ranges, so a remote image cannot be used to probe the reader's network.
func refusePrivateAddress(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("app: refusing to connect to %q", address)
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("app: refusing to proxy a request to the private address %s", ip)
	}
	return nil
}

func messageIDFromPath(path, prefix string) (int64, error) {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.Trim(rest, "/")
	if rest == "" || strings.Contains(rest, "/") {
		return 0, fmt.Errorf("app: malformed path %q", path)
	}
	return strconv.ParseInt(rest, 10, 64)
}

func assetRefFromPath(path string) (int64, string, error) {
	rest := strings.Trim(strings.TrimPrefix(path, assetPath), "/")
	idPart, token, found := strings.Cut(rest, "/")
	if !found || token == "" || strings.Contains(token, "/") {
		return 0, "", fmt.Errorf("app: malformed asset path %q", path)
	}
	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil {
		return 0, "", err
	}
	return id, token, nil
}

// locateMessage resolves a message id to the message, its folder and its
// account.
func (s *MailService) locateMessage(ctx context.Context, messageID int64) (model.Message, model.Folder, model.Account, error) {
	var folderID int64
	row := s.store.Read().QueryRowContext(ctx,
		`SELECT folder_id FROM messages WHERE id = ?`, messageID)
	if err := row.Scan(&folderID); err != nil {
		return model.Message{}, model.Folder{}, model.Account{},
			fmt.Errorf("app: no message with id %d: %w", messageID, err)
	}

	folder, acct, err := s.locateFolder(ctx, folderID)
	if err != nil {
		return model.Message{}, model.Folder{}, model.Account{}, err
	}

	var m model.Message
	var uid uint32
	row = s.store.Read().QueryRowContext(ctx,
		`SELECT uid FROM messages WHERE id = ?`, messageID)
	if err := row.Scan(&uid); err != nil {
		return model.Message{}, model.Folder{}, model.Account{}, err
	}
	m.ID = messageID
	m.UID = uid
	m.FolderID = folderID
	m.AccountID = acct.ID

	return m, folder, acct, nil
}

func wrapDocument(bodyHTML string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><style>` +
		`html,body{margin:0;padding:16px;font:14px/1.5 system-ui,sans-serif;color:#111;background:#fff}` +
		`img{max-width:100%;height:auto}table{max-width:100%}a{color:#1a56db}` +
		`@media (prefers-color-scheme: dark){html,body{color:#e5e5e5;background:#0a0a0a}a{color:#7aa2f7}}` +
		`</style></head><body>` + bodyHTML + `</body></html>`
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
