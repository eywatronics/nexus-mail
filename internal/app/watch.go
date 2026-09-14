package app

import (
	"context"

	"nexusmail/internal/model"
	imapsync "nexusmail/internal/sync"
)

// StartWatching begins live sync for every account, and for every account
// added afterwards. It returns as soon as the watchers are running.
//
// Without this call the whole IDLE machinery is dead code: the engine can
// watch an account, but nothing ever asks it to. Cancelling ctx stops every
// watcher and closes its connection, which is what the window closing does.
//
// Calling it twice is harmless. It has to be, because Gmail locks an account
// that opens too many connections, and a second set of watchers would double
// them.
func (s *MailService) StartWatching(ctx context.Context) error {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()

	if s.watchCtx != nil {
		return nil
	}
	s.watchCtx = ctx
	s.watching = map[int64]context.CancelFunc{}

	accounts, err := s.store.ListAccounts(ctx)
	if err != nil {
		return err
	}
	for _, acct := range accounts {
		s.startWatcherLocked(acct)
	}
	return nil
}

// watchAccount starts live sync for one account, if watching is running at all.
//
// Called after a successful first sync rather than at account creation: a
// brand new account has no folders yet, so a watcher started there would find
// no inbox and give up immediately.
func (s *MailService) watchAccount(acct model.Account) {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	s.startWatcherLocked(acct)
}

func (s *MailService) startWatcherLocked(acct model.Account) {
	if s.watchCtx == nil || s.watchCtx.Err() != nil {
		return
	}
	if _, running := s.watching[acct.ID]; running {
		return
	}

	ctx, cancel := context.WithCancel(s.watchCtx)
	s.watching[acct.ID] = cancel

	go func() {
		defer func() {
			cancel()
			s.watchMu.Lock()
			delete(s.watching, acct.ID)
			s.watchMu.Unlock()
		}()

		err := s.engine.Watch(ctx, acct, func() {
			// The pass already wrote whatever changed; this only tells the
			// window to read it again.
			s.cfg.Emit(EventSyncFinished, SyncEvent{AccountID: acct.ID, Email: acct.Email})
			// And, separately, whether any of it was worth interrupting the
			// reader for. Most passes are not: the loop wakes on flag changes
			// and expunges too, and only arrivals are news.
			s.announceNewMail(acct)
		})
		if err != nil && ctx.Err() == nil {
			s.cfg.Emit(EventSyncFailed, SyncEvent{
				AccountID: acct.ID,
				Email:     acct.Email,
				Error:     err.Error(),
				Class:     imapsync.Classify(err).String(),
			})
		}
	}()
}

// watcherCount reports how many watchers are running. Used by tests; the
// goroutines are otherwise invisible from outside.
func (s *MailService) watcherCount() int {
	s.watchMu.Lock()
	defer s.watchMu.Unlock()
	return len(s.watching)
}
