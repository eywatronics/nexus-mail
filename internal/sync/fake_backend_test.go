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

func newFakeBackend() *fakeBackend {
	return &fakeBackend{
		caps:        imapx.Capabilities{CondStore: true, QResync: true, Move: true, Idle: true},
		uidValidity: map[string]uint32{},
		messages:    map[string][]model.Message{},
		bodies:      map[uint32]imapx.Body{},
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
	}
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
	return imapx.SelectResult{
		UIDValidity:   v,
		UIDNext:       next,
		NumMessages:   uint32(len(f.messages[path])),
		HighestModSeq: 1,
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
