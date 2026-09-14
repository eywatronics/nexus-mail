package imapx

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"nexusmail/internal/auth"
	"nexusmail/internal/model"
)

type client struct {
	c    *imapclient.Client
	caps Capabilities

	// changes carries a signal from the server's unilateral responses to
	// whoever is sitting in Idle. Buffered to one and written without
	// blocking: several announcements in a row still mean one thing to us,
	// "something moved, go and look", and a blocking write here would stall
	// go-imap's reader goroutine.
	changes chan struct{}
}

// Dial connects, authenticates with the provider's SASL client and records the
// server's capabilities.
func Dial(ctx context.Context, cfg Config, provider auth.CredentialProvider) (MailBackend, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	changes := make(chan struct{}, 1)
	notify := func() {
		select {
		case changes <- struct{}{}:
		default: // a signal is already pending; one is enough
		}
	}

	// Only the responses that mean "the mailbox is not what you last saw" are
	// wired up. The Fetch handler is deliberately left nil: go-imap requires
	// whoever takes it to fully consume the message data, and we do not want
	// the message here — we want to know to run a delta pass, which will
	// fetch properly.
	opts := &imapclient.Options{
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Expunge: func(uint32) { notify() },
			Mailbox: func(*imapclient.UnilateralDataMailbox) { notify() },
		},
	}

	var (
		c   *imapclient.Client
		err error
	)
	if cfg.TLS {
		c, err = imapclient.DialTLS(addr, opts)
	} else {
		c, err = imapclient.DialInsecure(addr, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("imapx: dial %s: %w", addr, err)
	}

	saslClient, err := provider.SASLClient(ctx)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("imapx: obtaining credentials for %s: %w", cfg.Username, err)
	}
	if err := c.Authenticate(saslClient); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("imapx: authentication failed for %s: %w", cfg.Username, err)
	}

	// Capabilities are re-read after authentication: servers commonly
	// advertise a reduced set before login.
	caps, err := c.Capability().Wait()
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("imapx: reading capabilities: %w", err)
	}

	return &client{
		c:       c,
		changes: changes,
		caps: Capabilities{
			CondStore: caps.Has(imap.CapCondStore),
			QResync:   caps.Has(imap.CapQResync),
			Move:      caps.Has(imap.CapMove),
			Idle:      caps.Has(imap.CapIdle),
		},
	}, nil
}

func (cl *client) Capabilities() Capabilities { return cl.caps }

func (cl *client) Close() error { return cl.c.Close() }

// ListFolders issues LIST with STATUS return data, so counts and UIDVALIDITY
// arrive in the same round trip instead of one STATUS command per mailbox.
// On a corporate account with dozens of folders that is the difference
// between one exchange and eighty.
func (cl *client) ListFolders(_ context.Context) ([]model.Folder, error) {
	opts := &imap.ListOptions{
		ReturnStatus: &imap.StatusOptions{
			NumMessages: true,
			NumUnseen:   true,
			UIDNext:     true,
			UIDValidity: true,
		},
	}

	mailboxes, err := cl.c.List("", "*", opts).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapx: LIST failed: %w", err)
	}

	out := make([]model.Folder, 0, len(mailboxes))
	for _, mb := range mailboxes {
		delim := string(mb.Delim)
		f := model.Folder{
			Path:      mb.Mailbox,
			Name:      leafName(mb.Mailbox, delim),
			Delimiter: delim,
		}
		for _, attr := range mb.Attrs {
			f.Attributes = append(f.Attributes, string(attr))
		}
		if st := mb.Status; st != nil {
			if st.NumMessages != nil {
				f.TotalCount = int(*st.NumMessages)
			}
			if st.NumUnseen != nil {
				f.UnreadCount = int(*st.NumUnseen)
			}
			f.UIDNext = uint32(st.UIDNext)
			f.UIDValidity = st.UIDValidity
		}
		out = append(out, f)
	}
	return out, nil
}

func (cl *client) Select(_ context.Context, path string) (SelectResult, error) {
	data, err := cl.c.Select(path, nil).Wait()
	if err != nil {
		return SelectResult{}, fmt.Errorf("imapx: SELECT %q failed: %w", path, err)
	}
	return SelectResult{
		UIDValidity:   data.UIDValidity,
		UIDNext:       uint32(data.UIDNext),
		NumMessages:   data.NumMessages,
		HighestModSeq: data.HighestModSeq,
	}, nil
}

// FetchHeaders pulls envelopes, flags, sizes and body structures for a UID
// range. It deliberately does not fetch bodies: across a thousand-message
// initial sync that is the difference between kilobytes and tens of megabytes.
func (cl *client) FetchHeaders(_ context.Context, r UIDRange) ([]model.Message, error) {
	set := imap.UIDSet{imap.UIDRange{
		Start: imap.UID(r.Start),
		Stop:  imap.UID(r.End), // zero means "to the end", which matches UIDRange
	}}

	opts := &imap.FetchOptions{
		UID:           true,
		Flags:         true,
		Envelope:      true,
		InternalDate:  true,
		RFC822Size:    true,
		BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
	}

	buffers, err := cl.c.Fetch(set, opts).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapx: FETCH headers failed: %w", err)
	}

	out := make([]model.Message, 0, len(buffers))
	for _, buf := range buffers {
		m := model.Message{
			UID:          uint32(buf.UID),
			InternalDate: buf.InternalDate,
			Size:         buf.RFC822Size,
		}
		for _, f := range buf.Flags {
			m.Flags = append(m.Flags, string(f))
		}
		if env := buf.Envelope; env != nil {
			m.Subject = env.Subject
			m.MessageID = env.MessageID
			m.InReplyTo = strings.Join(env.InReplyTo, " ")
			m.From = firstAddress(env.From)
			m.To = addressesFrom(env.To)
			m.Cc = addressesFrom(env.Cc)
			// The Date: header is attacker-controlled and often malformed, so
			// INTERNALDATE wins whenever it is unusable or implausible.
			m.Date = reconcileDate(env.Date, buf.InternalDate)
		} else {
			m.Date = buf.InternalDate
		}
		if buf.BodyStructure != nil {
			m.HasAttachments = hasAttachmentParts(buf.BodyStructure)
		}
		m.ThreadID = ThreadKey(m.MessageID, m.InReplyTo, m.References, m.Subject)
		out = append(out, m)
	}
	return out, nil
}

// FetchBody pulls one whole message and extracts its text and HTML parts.
// Idle waits for the server to say the selected mailbox changed.
//
// Returns true when the server announced something, or ctx.Err() when the
// caller gave up. There is no timeout parameter on purpose: go-imap already
// restarts the IDLE command every 28 minutes internally, which is the RFC 2177
// requirement that would otherwise have to live here. A second restart timer
// on top of that would close and reopen the command for no reason. A caller
// that wants its own ceiling puts it on the context.
func (cl *client) Idle(ctx context.Context) (bool, error) {
	if !cl.caps.Idle {
		return false, fmt.Errorf("imapx: server does not support IDLE")
	}

	idle, err := cl.c.Idle()
	if err != nil {
		return false, fmt.Errorf("imapx: starting IDLE: %w", err)
	}

	var changed bool
	select {
	case <-cl.changes:
		changed = true
	case <-ctx.Done():
		_ = idle.Close()
		return false, ctx.Err()
	}

	// Close reports a connection that died while we were waiting, which is the
	// signal the supervisor needs to reconnect rather than loop.
	if err := idle.Close(); err != nil {
		return false, fmt.Errorf("imapx: ending IDLE: %w", err)
	}
	return changed, nil
}

// FetchFlags asks only for UIDs and flags.
//
// Two jobs in one call. With CONDSTORE (changedSince non-zero) the server
// answers with just what changed, which is what makes a delta sync cheap on a
// mailbox with fifty thousand messages. Without it, the full range comes back
// and the returned UID list is also the set of messages that still exist —
// the caller diffs it against the local set to find what was expunged.
//
// ModSeq is deliberately not requested. The caller records the HIGHESTMODSEQ
// that SELECT reported, and only after the whole pass succeeded; a per-message
// value would tempt it into recording progress mid-fetch, and an interrupted
// pass would then skip everything it had not reached.
func (cl *client) FetchFlags(_ context.Context, r UIDRange, changedSince uint64) ([]model.FlagUpdate, error) {
	set := imap.UIDSet{imap.UIDRange{
		Start: imap.UID(r.Start),
		Stop:  imap.UID(r.End), // zero means "to the end", which matches UIDRange
	}}

	buffers, err := cl.c.Fetch(set, &imap.FetchOptions{
		UID:          true,
		Flags:        true,
		ChangedSince: changedSince,
	}).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapx: FETCH flags failed: %w", err)
	}

	out := make([]model.FlagUpdate, 0, len(buffers))
	for _, buf := range buffers {
		// A message with no flags is a real state, and the store writes it as
		// an empty list rather than null. Starting from a non-nil slice keeps
		// that distinction from depending on whether the loop below ran.
		u := model.FlagUpdate{UID: uint32(buf.UID), Flags: []string{}}
		for _, f := range buf.Flags {
			u.Flags = append(u.Flags, string(f))
		}
		out = append(out, u)
	}
	return out, nil
}

func (cl *client) FetchBody(_ context.Context, uid uint32) (Body, error) {
	set := imap.UIDSet{imap.UIDRange{Start: imap.UID(uid), Stop: imap.UID(uid)}}
	opts := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{}},
	}

	buffers, err := cl.c.Fetch(set, opts).Collect()
	if err != nil {
		return Body{}, fmt.Errorf("imapx: FETCH body for UID %d failed: %w", uid, err)
	}
	if len(buffers) == 0 {
		return Body{}, fmt.Errorf("imapx: no message with UID %d", uid)
	}

	// One empty FetchItemBodySection was requested, so at most one section
	// comes back: the whole message.
	sections := buffers[0].BodySection
	if len(sections) == 0 {
		return Body{}, fmt.Errorf("imapx: message %d returned no body section", uid)
	}

	html, text, err := splitBodyParts(sections[0].Bytes)
	if err != nil {
		return Body{}, fmt.Errorf("imapx: parsing message %d: %w", uid, err)
	}
	return Body{HTML: html, Text: text}, nil
}

// leafName returns the display name of a mailbox path: "Parent/Child" becomes
// "Child". A mailbox with no delimiter is its own leaf.
func leafName(path, delim string) string {
	if delim == "" {
		return path
	}
	if i := strings.LastIndex(path, delim); i >= 0 {
		return path[i+len(delim):]
	}
	return path
}
