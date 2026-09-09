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
}

// Dial connects, authenticates with the provider's SASL client and records the
// server's capabilities.
func Dial(ctx context.Context, cfg Config, provider auth.CredentialProvider) (MailBackend, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))

	var (
		c   *imapclient.Client
		err error
	)
	if cfg.TLS {
		c, err = imapclient.DialTLS(addr, nil)
	} else {
		c, err = imapclient.DialInsecure(addr, nil)
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
		c: c,
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
