package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// OpenComposerWindow asks the host to put this draft in its own window and
// returns the one-time id that window uses to fetch it.
//
// The request is held here rather than passed in the URL: a URL ends up in
// window titles, crash reports and shell history, and a draft body does not
// belong in any of them. The id is random and taken exactly once.
func (s *MailService) OpenComposerWindow(d DraftRecordDTO) (string, error) {
	if d.AccountID == 0 {
		return "", fmt.Errorf("app: a composer window needs an account")
	}
	if s.cfg.OpenComposerWindow == nil {
		return "", fmt.Errorf("app: this build cannot open a separate window")
	}

	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("app: generating a window id: %w", err)
	}
	id := hex.EncodeToString(buf[:])

	s.windowMu.Lock()
	if s.composerWindow == nil {
		s.composerWindow = map[string]DraftRecordDTO{}
	}
	s.composerWindow[id] = d
	s.windowMu.Unlock()

	if err := s.cfg.OpenComposerWindow(id); err != nil {
		s.windowMu.Lock()
		delete(s.composerWindow, id)
		s.windowMu.Unlock()
		return "", err
	}
	return id, nil
}

// TakeComposerWindow hands the draft to the window that was opened for it.
//
// One shot: a reload of that window must not get a second copy and start a
// second composer over the same draft. A missing id is an error the window can
// show rather than an empty draft it could send.
func (s *MailService) TakeComposerWindow(id string) (DraftRecordDTO, error) {
	s.windowMu.Lock()
	defer s.windowMu.Unlock()

	d, ok := s.composerWindow[id]
	if !ok {
		return DraftRecordDTO{}, fmt.Errorf("app: this composer window has expired")
	}
	delete(s.composerWindow, id)
	return d, nil
}
