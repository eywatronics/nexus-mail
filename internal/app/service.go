// Package app exposes the application's capabilities to the frontend.
//
// It is the only layer the UI talks to, and it deliberately does not import
// Wails: event delivery arrives as a function, so every method here is
// unit-testable without a running window.
package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"nexusmail/internal/auth"
	"nexusmail/internal/imapx"
	"nexusmail/internal/model"
	"nexusmail/internal/store"
	imapsync "nexusmail/internal/sync"
)

// maxPageSize bounds what the frontend can ask for in one call. A runaway
// request would materialise the whole mailbox in memory on both sides of the
// bridge.
const maxPageSize = 500

type MailService struct {
	store   *store.Store
	secrets auth.SecretStore
	engine  *imapsync.Engine
	cfg     Config

	// Live sync state. watchCtx is the parent every watcher hangs off, stored
	// rather than passed because accounts are added long after StartWatching
	// was called and their watchers have to share the same lifetime.
	watchMu  sync.Mutex
	watchCtx context.Context
	watching map[int64]context.CancelFunc

	// arrivals is the highest message id each account has already been
	// announced. Guarded by watchMu, which is already the lock for everything
	// the watch goroutines touch.
	arrivals map[int64]int64
}

func NewMailService(s *store.Store, secrets auth.SecretStore, eng *imapsync.Engine, cfg Config) *MailService {
	if cfg.Emit == nil {
		cfg.Emit = func(string, any) {}
	}
	return &MailService{store: s, secrets: secrets, engine: eng, cfg: cfg}
}

func (s *MailService) ListAccounts() ([]AccountDTO, error) {
	accounts, err := s.store.ListAccounts(context.Background())
	if err != nil {
		return nil, err
	}
	out := make([]AccountDTO, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, accountToDTO(a))
	}
	return out, nil
}

// AddPasswordAccount registers an account authenticated with a password or app
// password. The secret goes straight to the SecretStore; only its ref reaches
// the database.
// AddPasswordAccount registers an account authenticated with a password or app
// password. The secret goes straight to the SecretStore; only its ref reaches
// the database.
//
// imapSecurity is "tls" or "starttls". Anything else, including empty, is read
// as implicit TLS — the stronger of the two, and the only safe direction to
// resolve an unrecognised value in. On-premises Exchange is the reason the
// choice exists: its IMAP4 service is usually published on 143, where the
// connection has to be upgraded before it will take a password.
func (s *MailService) AddPasswordAccount(email, displayName, imapHost string, imapPort int,
	imapSecurity string, smtpHost string, smtpPort int, password string) (AccountDTO, error) {

	if email == "" {
		return AccountDTO{}, errors.New("app: an email address is required")
	}
	if password == "" {
		return AccountDTO{}, errors.New("app: a password is required")
	}

	security := model.SecurityOrDefault(imapSecurity)

	provider := model.ProviderGeneric
	if preset, ok := auth.PresetFor(email); ok {
		provider = preset.Provider
		if imapHost == "" {
			imapHost, imapPort = preset.IMAPHost, preset.IMAPPort
			smtpHost, smtpPort = preset.SMTPHost, preset.SMTPPort
			// A preset names a host that answers on 993; taking the user's
			// STARTTLS choice with it would point the upgrade at a port that
			// has nothing to upgrade.
			security = model.SecurityTLS
		}
	}
	if imapHost == "" {
		return AccountDTO{}, fmt.Errorf(
			"app: no IMAP host given and no preset for %q; corporate servers must be entered manually", email)
	}
	if imapPort == 0 {
		imapPort = model.DefaultIMAPPort(security)
	}

	return s.saveAccount(model.Account{
		Email: email, DisplayName: displayName, Provider: provider,
		AuthKind: model.AuthPassword,
		IMAPHost: imapHost, IMAPPort: imapPort, IMAPSecurity: security,
		SMTPHost: smtpHost, SMTPPort: smtpPort,
	}, password)
}

// AddOAuthAccount runs the loopback flow, stores the refresh token and records
// the account. It blocks while the user completes sign-in in their browser.
func (s *MailService) AddOAuthAccount(email, displayName, providerName string) (AccountDTO, error) {
	if email == "" {
		return AccountDTO{}, errors.New("app: an email address is required")
	}

	provider := model.Provider(providerName)
	oauthCfg, err := s.oauthConfigFor(provider)
	if err != nil {
		return AccountDTO{}, err
	}

	// Five minutes is generous for a browser sign-in and still bounded: an
	// abandoned flow must not hold a listener open for the session.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	tok, err := auth.RunLoopbackFlow(ctx, oauthCfg)
	if err != nil {
		return AccountDTO{}, err
	}
	if tok.RefreshToken == "" {
		return AccountDTO{}, errors.New(
			"app: the provider returned no refresh token; offline access was not granted")
	}

	preset := s.serverPresetFor(email, provider)
	return s.saveAccount(model.Account{
		Email: email, DisplayName: displayName, Provider: provider,
		AuthKind: model.AuthOAuth,
		IMAPHost: preset.IMAPHost, IMAPPort: preset.IMAPPort,
		SMTPHost: preset.SMTPHost, SMTPPort: preset.SMTPPort,
	}, tok.RefreshToken)
}

// saveAccount stores the secret, then the row, rolling the secret back if the
// row cannot be written.
//
// The rollback only removes a secret this call created. The ref is derived
// from the address, so re-adding an existing account lands on the ref the
// working account already uses — deleting it unconditionally would destroy a
// live credential to clean up after a failure that changed nothing.
func (s *MailService) saveAccount(acct model.Account, secret string) (AccountDTO, error) {
	ctx := context.Background()

	acct.SecretRef = "account:" + acct.Email
	acct.CreatedAt = time.Now()

	// Check first so the user gets a clear message rather than a UNIQUE
	// constraint error. The constraint is still the real guard.
	if existing, err := s.store.ListAccounts(ctx); err == nil {
		for _, a := range existing {
			if a.Email == acct.Email {
				return AccountDTO{}, fmt.Errorf("app: %s has already been added", acct.Email)
			}
		}
	}

	_, getErr := s.secrets.Get(acct.SecretRef)
	secretPreexisted := getErr == nil

	if err := s.secrets.Set(acct.SecretRef, secret); err != nil {
		return AccountDTO{}, fmt.Errorf("app: storing credentials: %w", err)
	}

	id, err := s.store.InsertAccount(ctx, acct)
	if err != nil {
		if !secretPreexisted {
			if delErr := s.secrets.Delete(acct.SecretRef); delErr != nil {
				return AccountDTO{}, fmt.Errorf(
					"app: saving account failed (%w) and the stored credential could not be removed: %w",
					err, delErr)
			}
		}
		return AccountDTO{}, fmt.Errorf("app: saving account: %w", err)
	}

	acct.ID = id
	return accountToDTO(acct), nil
}

func (s *MailService) oauthConfigFor(provider model.Provider) (auth.OAuthConfig, error) {
	switch provider {
	case model.ProviderGoogle:
		if s.cfg.GoogleClientID == "" {
			return auth.OAuthConfig{}, errors.New(
				"app: no Google OAuth client ID configured; see docs/oauth-setup.md")
		}
		return auth.OAuthConfig{
			ClientID:     s.cfg.GoogleClientID,
			Scopes:       auth.GoogleScopes(),
			Endpoint:     auth.GoogleEndpoint(),
			RedirectPort: s.cfg.OAuthRedirectPort,
		}, nil
	case model.ProviderMicrosoft:
		if s.cfg.MicrosoftClientID == "" {
			return auth.OAuthConfig{}, errors.New(
				"app: no Microsoft OAuth client ID configured; see docs/oauth-setup.md")
		}
		return auth.OAuthConfig{
			ClientID:     s.cfg.MicrosoftClientID,
			Scopes:       auth.MicrosoftScopes(),
			Endpoint:     auth.MicrosoftEndpoint(),
			RedirectPort: s.cfg.OAuthRedirectPort,
		}, nil
	default:
		return auth.OAuthConfig{}, fmt.Errorf("app: provider %q does not support OAuth", provider)
	}
}

// serverPresetFor falls back to the provider's standard hosts. Work and school
// addresses use custom domains that are not in the preset table, but Exchange
// Online and Gmail both answer on one well-known host regardless of domain.
func (s *MailService) serverPresetFor(email string, provider model.Provider) auth.Preset {
	if preset, ok := auth.PresetFor(email); ok {
		return preset
	}
	if provider == model.ProviderGoogle {
		return auth.GmailPreset
	}
	return auth.ExchangeOnlinePreset
}

func (s *MailService) SyncAccount(accountID int64) error {
	ctx := context.Background()

	acct, err := s.store.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}

	s.cfg.Emit(EventSyncStarted, SyncEvent{AccountID: acct.ID, Email: acct.Email})

	if err := s.engine.InitialSync(ctx, acct); err != nil {
		s.cfg.Emit(EventSyncFailed, SyncEvent{
			AccountID: acct.ID,
			Email:     acct.Email,
			Error:     err.Error(),
			Class:     imapsync.Classify(err).String(),
		})
		return err
	}

	s.cfg.Emit(EventSyncFinished, SyncEvent{AccountID: acct.ID, Email: acct.Email})

	// Now that the account has folders, it is worth watching. Starting the
	// watcher when the account was created would have found no inbox.
	s.watchAccount(acct)
	return nil
}

func (s *MailService) ListFolders(accountID int64) ([]FolderDTO, error) {
	folders, err := s.store.ListFolders(context.Background(), accountID)
	if err != nil {
		return nil, err
	}
	out := make([]FolderDTO, 0, len(folders))
	for _, f := range folders {
		out = append(out, folderToDTO(f))
	}
	return out, nil
}

// OpenFolder fetches headers for a folder the initial sync left empty, then
// returns its first page. Ordinary folders are lazy, so this is what the UI
// calls when the user clicks one.
func (s *MailService) OpenFolder(folderID int64, limit int, threaded bool) ([]MessageDTO, error) {
	ctx := context.Background()

	folder, acct, err := s.locateFolder(ctx, folderID)
	if err != nil {
		return nil, err
	}

	if folder.LastSyncedAt.IsZero() {
		if err := s.engine.SyncFolder(ctx, acct, folder); err != nil {
			s.cfg.Emit(EventSyncFailed, SyncEvent{
				AccountID: acct.ID, Email: acct.Email,
				Error: err.Error(), Class: imapsync.Classify(err).String(),
			})
			return nil, err
		}
	}
	return s.ListMessages(folderID, limit, 0, threaded)
}

// ListMessages returns one page of a folder.
//
// threaded changes the order, not the contents: conversations come back with
// their messages adjacent and are ranked by their newest, so the window can
// group runs of adjacent rows. It is a parameter rather than two methods
// because it is one question — what order — and the caller answers it per
// request; the reader can switch views without anything being resynced.
func (s *MailService) ListMessages(folderID int64, limit, offset int, threaded bool) ([]MessageDTO, error) {
	if limit <= 0 || limit > maxPageSize {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	ctx := context.Background()
	list := s.store.ListMessages
	if threaded {
		list = s.store.ListThreadedMessages
	}

	msgs, err := list(ctx, folderID, limit, offset)
	if err != nil {
		return nil, err
	}
	out := make([]MessageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, messageToDTO(m))
	}
	return out, nil
}

// SearchMessages finds messages across all of an account's folders.
//
// Search deliberately spans folders while the list does not. Someone reaching
// for search has already failed to find the message by browsing, and the most
// common reason is that it is not where they expected. Scoping results to the
// folder they happen to be standing in would reproduce the failure.
//
// Only synced headers are searchable. A folder nobody has opened has no rows
// yet, so its mail cannot appear here — the honest consequence of syncing
// lazily, and the reason OpenFolder exists.
func (s *MailService) SearchMessages(accountID int64, query string, limit int) ([]MessageDTO, error) {
	if limit <= 0 || limit > maxPageSize {
		limit = 100
	}

	msgs, err := s.store.SearchMessages(context.Background(), accountID, query, limit)
	if err != nil {
		return nil, err
	}

	out := make([]MessageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, messageToDTO(m))
	}
	return out, nil
}

func (s *MailService) locateFolder(ctx context.Context, folderID int64) (model.Folder, model.Account, error) {
	var accountID int64
	row := s.store.Read().QueryRowContext(ctx,
		`SELECT account_id FROM folders WHERE id = ?`, folderID)
	if err := row.Scan(&accountID); err != nil {
		return model.Folder{}, model.Account{},
			fmt.Errorf("app: no folder with id %d: %w", folderID, err)
	}

	acct, err := s.store.GetAccount(ctx, accountID)
	if err != nil {
		return model.Folder{}, model.Account{}, err
	}

	folders, err := s.store.ListFolders(ctx, accountID)
	if err != nil {
		return model.Folder{}, model.Account{}, err
	}
	for _, f := range folders {
		if f.ID == folderID {
			return f, acct, nil
		}
	}
	return model.Folder{}, model.Account{},
		fmt.Errorf("app: folder %d vanished between lookups", folderID)
}

// DialerFor builds the sync engine's dialer: it resolves an account to a
// credential provider and opens an IMAP connection.
//
// This lives in app rather than in sync because it is where the three auth
// paths converge, and sync must stay unaware that more than one exists.
func DialerFor(s *store.Store, secrets auth.SecretStore, cfg Config) imapsync.Dialer {
	return func(ctx context.Context, accountID int64) (imapx.MailBackend, error) {
		acct, err := s.GetAccount(ctx, accountID)
		if err != nil {
			return nil, err
		}

		var provider auth.CredentialProvider
		switch acct.AuthKind {
		case model.AuthPassword:
			provider = auth.NewPasswordProvider(acct.Email, acct.SecretRef, secrets)
		case model.AuthOAuth:
			oauthCfg := auth.OAuthConfig{
				ClientID: cfg.GoogleClientID,
				Scopes:   auth.GoogleScopes(),
				Endpoint: auth.GoogleEndpoint(),
			}
			if acct.Provider == model.ProviderMicrosoft {
				oauthCfg = auth.OAuthConfig{
					ClientID: cfg.MicrosoftClientID,
					Scopes:   auth.MicrosoftScopes(),
					Endpoint: auth.MicrosoftEndpoint(),
				}
			}
			provider = auth.NewOAuthProvider(acct.Email, acct.SecretRef, secrets, oauthCfg)
		default:
			return nil, fmt.Errorf("app: unknown auth kind %q for account %d", acct.AuthKind, accountID)
		}

		// Encryption is chosen between implicit TLS and STARTTLS, never
		// switched off: the cleartext path exists only for the loopback test
		// server and imapx refuses it for anything else.
		return imapx.Dial(ctx, imapx.Config{
			Host:     acct.IMAPHost,
			Port:     acct.IMAPPort,
			Security: acct.IMAPSecurity,
			Username: acct.Email,
		}, provider)
	}
}
