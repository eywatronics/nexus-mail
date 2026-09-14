package imapx

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/quotedprintable"
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

	// Checked before a socket is opened rather than after: this is the one
	// rule that, broken, puts a password on the wire, and the earliest place
	// to enforce it is before there is a wire.
	if err := checkSecurity(cfg); err != nil {
		return nil, err
	}

	c, err := connect(addr, cfg, opts)
	if err != nil {
		return nil, err
	}

	if err := authenticate(ctx, c, provider); err != nil {
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
			CondStore:  caps.Has(imap.CapCondStore),
			QResync:    caps.Has(imap.CapQResync),
			Move:       caps.Has(imap.CapMove),
			Idle:       caps.Has(imap.CapIdle),
			UIDPlus:    caps.Has(imap.CapUIDPlus),
			SpecialUse: caps.Has(imap.CapSpecialUse),
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
		// Asked for explicitly rather than hoped for. RFC 6154 only says a
		// server SHOULD volunteer the special-use attributes in a plain LIST,
		// and the ones that do not are precisely the ones where guessing from
		// the folder name goes wrong.
		ReturnSpecialUse: cl.caps.SpecialUse,
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
			// Listing the parts here costs nothing: the BODYSTRUCTURE is
			// already in hand. Asking again later, per message, when the
			// reader opens one, would be a round trip for data we threw away.
			m.Attachments = attachmentParts(buf.BodyStructure)
		}
		m.ThreadID = ThreadKey(m.MessageID, m.InReplyTo, m.References, m.Subject)
		out = append(out, m)
	}
	return out, nil
}

// FetchBody pulls one whole message and extracts its text and HTML parts.
// FetchPart returns the decoded bytes of one part of a message.
//
// Separate from FetchBody because an attachment is not wanted until somebody
// asks for it. A twenty-megabyte PDF pulled down with every header sync would
// undo the point of syncing headers.
//
// The transfer encoding is undone here rather than by the caller: what comes
// off the wire is base64 or quoted-printable, and handing that to a file
// dialog would save a file nothing can open.
func (cl *client) FetchPart(_ context.Context, uid uint32, partID, encoding string) ([]byte, error) {
	if partID == "" {
		return nil, fmt.Errorf("imapx: no part number given")
	}

	section := &imap.FetchItemBodySection{Part: parsePartNumber(partID), Peek: true}
	buffers, err := cl.c.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapx: FETCH part %s failed: %w", partID, err)
	}
	if len(buffers) == 0 {
		return nil, fmt.Errorf("imapx: no message with UID %d", uid)
	}

	raw := buffers[0].FindBodySection(section)
	if raw == nil {
		return nil, fmt.Errorf("imapx: message %d has no part %s", uid, partID)
	}

	decoded, err := decodePart(raw, encoding)
	if err != nil {
		return nil, fmt.Errorf("imapx: decoding part %s of message %d: %w", partID, uid, err)
	}
	return decoded, nil
}

// decodePart undoes the transfer encoding the server reported.
//
// An unknown encoding is passed through rather than refused: the alternative
// is failing to open a file that is probably fine, over a header some sending
// client got wrong.
func decodePart(raw []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "base64":
		// Servers wrap base64 at 76 characters and the line breaks are not
		// part of the alphabet, so the decoder has to be fed without them.
		return io.ReadAll(base64.NewDecoder(base64.StdEncoding,
			bytes.NewReader(stripLineBreaks(raw))))
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(bytes.NewReader(raw)))
	default:
		return raw, nil
	}
}

// stripLineBreaks removes CR and LF, leaving everything else alone.
func stripLineBreaks(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	for _, b := range raw {
		if b != 13 && b != 10 {
			out = append(out, b)
		}
	}
	return out
}

// parsePartNumber turns "1.3" back into the path go-imap wants.
func parsePartNumber(partID string) []int {
	var path []int
	for _, chunk := range strings.Split(partID, ".") {
		n, err := strconv.Atoi(chunk)
		if err != nil {
			return nil
		}
		path = append(path, n)
	}
	return path
}

// StoreFlags adds or removes flags on a UID set.
//
// Add and remove rather than set: two clients touching the same message should
// not undo each other, and SET would replace the whole flag list with ours,
// wiping anything the other one added.
//
// Both directions are idempotent, which is what makes the outgoing queue safe
// to retry. Adding a flag that is already there changes nothing.
func (cl *client) StoreFlags(_ context.Context, uids []uint32, flags []string, add bool) error {
	set, err := uidSet(uids)
	if err != nil {
		return err
	}

	op := imap.StoreFlagsDel
	if add {
		op = imap.StoreFlagsAdd
	}

	converted := make([]imap.Flag, 0, len(flags))
	for _, f := range flags {
		converted = append(converted, imap.Flag(f))
	}

	// Silent: the server would otherwise answer with an untagged FETCH per
	// message, which we would throw away — the next delta pass is what reads
	// the resulting state.
	cmd := cl.c.Store(set, &imap.StoreFlags{
		Op: op, Flags: converted, Silent: true,
	}, nil)
	if _, err := cmd.Collect(); err != nil {
		return fmt.Errorf("imapx: STORE flags failed: %w", err)
	}
	return nil
}

// Move relocates messages to another mailbox.
//
// go-imap falls back to COPY plus STORE plus EXPUNGE on servers without the
// MOVE extension, so the caller does not have to care which it is talking to.
func (cl *client) Move(_ context.Context, uids []uint32, destPath string) error {
	set, err := uidSet(uids)
	if err != nil {
		return err
	}
	if destPath == "" {
		return fmt.Errorf("imapx: MOVE needs a destination mailbox")
	}

	if _, err := cl.c.Move(set, destPath).Wait(); err != nil {
		return fmt.Errorf("imapx: MOVE to %q failed: %w", destPath, err)
	}
	return nil
}

// Expunge permanently removes the named messages.
//
// Two steps, because IMAP has no single "delete this message" command: flag
// them deleted, then expunge.
//
// The expunge is scoped with UIDPLUS where the server has it. Where it does
// not, the messages are left flagged and nothing is expunged — deliberately.
// The only other EXPUNGE available removes *every* message in the mailbox
// flagged deleted, including ones flagged by another client or left over from
// a session that never finished, and destroying someone else's mail to carry
// out our own delete is not a trade worth making. The flagged messages
// disappear from every client's view and go on the server's next expunge.
func (cl *client) Expunge(_ context.Context, uids []uint32) error {
	set, err := uidSet(uids)
	if err != nil {
		return err
	}

	cmd := cl.c.Store(set, &imap.StoreFlags{
		Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagDeleted}, Silent: true,
	}, nil)
	if _, err := cmd.Collect(); err != nil {
		return fmt.Errorf("imapx: flagging messages deleted failed: %w", err)
	}

	if !cl.caps.UIDPlus {
		return nil
	}
	if _, err := cl.c.UIDExpunge(set).Collect(); err != nil {
		return fmt.Errorf("imapx: UID EXPUNGE failed: %w", err)
	}
	return nil
}

// uidSet converts our UID list into go-imap's set type, refusing an empty one.
//
// An empty set is a command that names nothing: some servers answer with an
// error, others with something surprising, and neither is what the caller
// meant to ask.
func uidSet(uids []uint32) (imap.UIDSet, error) {
	if len(uids) == 0 {
		return nil, fmt.Errorf("imapx: no UIDs given")
	}
	var set imap.UIDSet
	for _, uid := range uids {
		set.AddNum(imap.UID(uid))
	}
	return set, nil
}

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

// FetchBody returns the renderable parts of one message.
//
// Peek is set, and that is not an optimisation. A plain BODY[] fetch sets the
// \Seen flag as a protocol side effect, so rendering a message in the reading
// pane would mark it read on every other device — without passing through the
// outgoing queue, so this app would not even know it had happened. Whether a
// message counts as read is the reader's decision; BODY.PEEK is what keeps it
// one.
func (cl *client) FetchBody(_ context.Context, uid uint32) (Body, error) {
	set := imap.UIDSet{imap.UIDRange{Start: imap.UID(uid), Stop: imap.UID(uid)}}
	opts := &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{{Peek: true}},
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

// FetchRaw returns the message exactly as it sits on the server, headers and
// all.
//
// Not reassembled from the parsed parts. "View source" and "save as .eml" are
// both asking the same question — what did this machine actually receive — and
// an answer this client had rewritten would not be evidence of anything. It is
// also the input the charset repair needs, because repairing means decoding
// the original bytes differently.
func (cl *client) FetchRaw(_ context.Context, uid uint32) ([]byte, error) {
	section := &imap.FetchItemBodySection{Peek: true}
	buffers, err := cl.c.Fetch(imap.UIDSetNum(imap.UID(uid)), &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{section},
	}).Collect()
	if err != nil {
		return nil, fmt.Errorf("imapx: FETCH raw message %d failed: %w", uid, err)
	}
	if len(buffers) == 0 {
		return nil, fmt.Errorf("imapx: no message with UID %d", uid)
	}

	raw := buffers[0].FindBodySection(section)
	if raw == nil {
		return nil, fmt.Errorf("imapx: message %d returned no body section", uid)
	}
	return raw, nil
}

// checkSecurity refuses a cleartext connection to anything but loopback.
//
// Insecure exists for one reason: the in-memory IMAP server the engine is
// tested against speaks no TLS. That server is always on 127.0.0.1, so the
// legitimate use is fully described by "loopback" — and every other use of the
// flag is a mistake that would send a password across a network in the clear.
//
// A comment saying "tests only" does not survive the first person who copies
// the struct literal. This does.
func checkSecurity(cfg Config) error {
	if !cfg.Insecure {
		return nil
	}
	if isLoopback(cfg.Host) {
		return nil
	}
	return fmt.Errorf(
		"imapx: refusing an unencrypted connection to %q; plaintext is for the "+
			"loopback test server and nothing else", cfg.Host)
}

func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

// connect opens the transport the configuration asks for.
//
// STARTTLS is mandatory rather than opportunistic. go-imap's DialStartTLS
// fails if the server does not offer the upgrade, which is the behaviour we
// want: a network attacker able to strip the STARTTLS advertisement from the
// greeting would otherwise be handed the password, and that attack is the
// reason opportunistic encryption is not encryption.
func connect(addr string, cfg Config, opts *imapclient.Options) (*imapclient.Client, error) {
	var (
		c   *imapclient.Client
		err error
	)
	switch {
	case cfg.Insecure:
		c, err = imapclient.DialInsecure(addr, opts)
	case cfg.Security == model.SecuritySTARTTLS:
		c, err = imapclient.DialStartTLS(addr, opts)
	default:
		c, err = imapclient.DialTLS(addr, opts)
	}
	if err != nil {
		return nil, fmt.Errorf("imapx: dial %s: %w", addr, describeDialError(cfg, err))
	}
	return c, nil
}

// describeDialError adds what the operator needs in order to fix the two
// mistakes this step actually fails on.
//
// A certificate error on an internal server is almost never a real attack; it
// is an internal CA the machine does not trust, and the generic Go message
// sends people looking in the wrong place. Choosing the wrong port is the
// other one: 993 speaks TLS from the first byte and 143 does not, so pointing
// implicit TLS at 143 produces a handshake failure that says nothing about
// ports.
func describeDialError(cfg Config, err error) error {
	var certErr x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	switch {
	case errors.As(err, &certErr):
		return fmt.Errorf("%w — the server's certificate was issued by an authority "+
			"this machine does not trust, which on an internal server usually means "+
			"the organisation's root certificate is not installed", err)
	case errors.As(err, &hostErr):
		return fmt.Errorf("%w — the certificate does not name this host; check the "+
			"server name against the one the certificate was issued for", err)
	case cfg.Security != model.SecuritySTARTTLS && cfg.Port == 143:
		return fmt.Errorf("%w — port 143 does not speak TLS from the first byte; "+
			"this account is set to implicit TLS, which belongs on 993", err)
	case cfg.Security == model.SecuritySTARTTLS && cfg.Port == 993:
		return fmt.Errorf("%w — port 993 is encrypted from the first byte and has no "+
			"STARTTLS to negotiate; this account is set to STARTTLS, which belongs "+
			"on 143", err)
	default:
		return err
	}
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
