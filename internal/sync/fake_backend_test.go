package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
)

// fakeBackend is a scriptable MailBackend.
//
// The imapx package already tests against a real in-memory IMAP server. Here
// we need to drive specific server *behaviours* — a UIDVALIDITY bump, a
// mid-sync failure, a server without CONDSTORE — which is easier to arrange
// directly than to coax out of a real one. imapmemserver in particular cannot
// be made to advertise CONDSTORE at all, so that path only exists here.
type fakeBackend struct {
	mu sync.Mutex

	caps     imapx.Capabilities
	folders  []model.Folder
	selected string

	uidValidity map[string]uint32
	messages    map[string][]model.Message
	bodies      map[uint32]imapx.Body

	// modseq tracks a CONDSTORE modification sequence per message, and
	// highestModSeq what SELECT reports. A real server bumps these on every
	// change; the fake bumps them wherever a test changes something.
	modseq        map[string]map[uint32]uint64
	highestModSeq map[string]uint64

	// flagFetches records what FetchFlags was asked for. The only observable
	// difference between the CONDSTORE path and the fallback is the argument
	// the engine passes, so a test has to be able to look at it.
	flagFetches []flagFetch

	// failFetchAfter makes FetchHeaders fail once it has served this many
	// messages, simulating a connection drop mid-sync. Zero disables it.
	failFetchAfter int
	served         int

	// fetchErr overrides the error a failing fetch returns, so a test can
	// exercise a specific error class.
	fetchErr error

	selectCalls int
	closed      bool
}

// flagFetch is one recorded call to FetchFlags.
type flagFetch struct {
	path         string
	rng          imapx.UIDRange
	changedSince uint64
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		caps:          imapx.Capabilities{CondStore: true, QResync: true, Move: true, Idle: true},
		uidValidity:   map[string]uint32{},
		messages:      map[string][]model.Message{},
		bodies:        map[uint32]imapx.Body{},
		modseq:        map[string]map[uint32]uint64{},
		highestModSeq: map[string]uint64{},
	}
}

func (f *fakeBackend) addFolder(path string, attrs []string, uidValidity uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.folders = append(f.folders, model.Folder{
		Path: path, Name: path, Delimiter: "/", Attributes: attrs,
		UIDValidity: uidValidity,
	})
	f.uidValidity[path] = uidValidity
}

func (f *fakeBackend) addMessages(path string, uids ...uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, uid := range uids {
		id := fmt.Sprintf("<%s-%d@example.com>", path, uid)
		f.messages[path] = append(f.messages[path], model.Message{
			UID:          uid,
			MessageID:    id,
			ThreadID:     id,
			Subject:      fmt.Sprintf("%s message %d", path, uid),
			From:         model.Address{Name: "Sender", Addr: "sender@example.com"},
			InternalDate: time.Unix(1700000000+int64(uid), 0),
			Date:         time.Unix(1700000000+int64(uid), 0),
			Size:         1024,
		})
		f.bodies[uid] = imapx.Body{
			HTML: fmt.Sprintf("<p>body %d</p>", uid),
			Text: fmt.Sprintf("body %d", uid),
		}
		f.bumpModSeqLocked(path, uid)
	}
}

// bumpModSeqLocked advances the mailbox's modification sequence and stamps one
// message with it, the way a server does on any change to that message.
func (f *fakeBackend) bumpModSeqLocked(path string, uid uint32) {
	f.highestModSeq[path]++
	if f.modseq[path] == nil {
		f.modseq[path] = map[uint32]uint64{}
	}
	f.modseq[path][uid] = f.highestModSeq[path]
}

// setFlags is the server-side change a delta sync is supposed to notice: a
// message read on a phone, a star added in a webmail.
func (f *fakeBackend) setFlags(path string, uid uint32, flags ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for i := range f.messages[path] {
		if f.messages[path][i].UID == uid {
			f.messages[path][i].Flags = flags
			f.bumpModSeqLocked(path, uid)
			return
		}
	}
}

// expunge removes messages server-side. Note it does NOT bump a modseq:
// CONDSTORE alone does not report deletions, which is exactly why the engine
// cannot rely on CHANGEDSINCE to find them.
func (f *fakeBackend) expunge(path string, uids ...uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()

	gone := map[uint32]bool{}
	for _, uid := range uids {
		gone[uid] = true
	}
	kept := f.messages[path][:0]
	for _, m := range f.messages[path] {
		if !gone[m.UID] {
			kept = append(kept, m)
		}
	}
	f.messages[path] = kept
}

// recordedFlagFetches returns a copy of what FetchFlags was asked for.
func (f *fakeBackend) recordedFlagFetches() []flagFetch {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]flagFetch, len(f.flagFetches))
	copy(out, f.flagFetches)
	return out
}

// setUIDValidity simulates the server recreating a mailbox: same name, brand
// new UID space.
func (f *fakeBackend) setUIDValidity(path string, v uint32) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.uidValidity[path] = v
	for i := range f.folders {
		if f.folders[i].Path == path {
			f.folders[i].UIDValidity = v
		}
	}
}

func (f *fakeBackend) clearMessages(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages[path] = nil
}

func (f *fakeBackend) Capabilities() imapx.Capabilities {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.caps
}

func (f *fakeBackend) ListFolders(_ context.Context) ([]model.Folder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]model.Folder, len(f.folders))
	copy(out, f.folders)
	for i := range out {
		out[i].TotalCount = len(f.messages[out[i].Path])
	}
	return out, nil
}

func (f *fakeBackend) Select(_ context.Context, path string) (imapx.SelectResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.selectCalls++
	v, ok := f.uidValidity[path]
	if !ok {
		return imapx.SelectResult{}, fmt.Errorf("SELECT %q failed: no such mailbox", path)
	}
	f.selected = path

	var next uint32 = 1
	for _, m := range f.messages[path] {
		if m.UID >= next {
			next = m.UID + 1
		}
	}
	// A server without CONDSTORE reports no modification sequence at all;
	// mirroring that is what lets a test drive the fallback path.
	var highest uint64
	if f.caps.CondStore {
		highest = f.highestModSeq[path]
	}
	return imapx.SelectResult{
		UIDValidity:   v,
		UIDNext:       next,
		NumMessages:   uint32(len(f.messages[path])),
		HighestModSeq: highest,
	}, nil
}

func (f *fakeBackend) FetchHeaders(_ context.Context, r imapx.UIDRange) ([]model.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	// A custom fetchErr with failFetchAfter left at zero fails on the very
	// first fetch, which is what an authentication failure looks like.
	if (f.fetchErr != nil || f.failFetchAfter > 0) && f.served >= f.failFetchAfter {
		if f.fetchErr != nil {
			return nil, f.fetchErr
		}
		return nil, fmt.Errorf("FETCH failed: connection reset by peer")
	}

	var out []model.Message
	for _, m := range f.messages[f.selected] {
		if m.UID < r.Start {
			continue
		}
		if r.End != 0 && m.UID > r.End {
			continue
		}
		out = append(out, m)
	}
	f.served += len(out)
	return out, nil
}

func (f *fakeBackend) FetchFlags(_ context.Context, r imapx.UIDRange, changedSince uint64) ([]model.FlagUpdate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.flagFetches = append(f.flagFetches, flagFetch{
		path: f.selected, rng: r, changedSince: changedSince,
	})

	if f.fetchErr != nil {
		return nil, f.fetchErr
	}

	var out []model.FlagUpdate
	for _, m := range f.messages[f.selected] {
		if m.UID < r.Start {
			continue
		}
		if r.End != 0 && m.UID > r.End {
			continue
		}
		// CHANGEDSINCE: the server sends only messages whose modseq moved
		// past the given value.
		if changedSince > 0 && f.modseq[f.selected][m.UID] <= changedSince {
			continue
		}
		flags := m.Flags
		if flags == nil {
			flags = []string{}
		}
		out = append(out, model.FlagUpdate{UID: m.UID, Flags: flags})
	}
	return out, nil
}

func (f *fakeBackend) FetchBody(_ context.Context, uid uint32) (imapx.Body, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	b, ok := f.bodies[uid]
	if !ok {
		return imapx.Body{}, fmt.Errorf("no message with UID %d", uid)
	}
	return b, nil
}

func (f *fakeBackend) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}
