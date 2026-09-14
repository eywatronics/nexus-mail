package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nexusmail/internal/model"
)

const configFileName = "config.json"

type fileConfig struct {
	GoogleClientID    string `json:"googleClientId"`
	MicrosoftClientID string `json:"microsoftClientId"`
	OAuthRedirectPort int    `json:"oauthRedirectPort"`

	// Pointers so that absent and zero are different answers. Zero is the
	// user turning a limit off, which on a local-first client has to be
	// something they can actually say; absent means "use the default".
	RetentionDays        *int `json:"retentionDays,omitempty"`
	RetentionMaxMessages *int `json:"retentionMaxMessages,omitempty"`

	// A pointer for the same reason: absent has to mean "the default", and the
	// default here is on. A plain bool would make a config file that never
	// mentioned notifications silently turn their content off.
	NotificationPreview *bool `json:"notificationPreview,omitempty"`
}

// Config carries wiring the service cannot construct for itself.
type Config struct {
	Emit Emitter

	// OAuth client IDs come from config.json. They are identifiers rather than
	// secrets: a desktop app is a public OAuth client and cannot keep a secret
	// on the user's machine. The refresh token is the sensitive part, and that
	// goes to the OS keyring.
	GoogleClientID    string
	MicrosoftClientID string

	// OAuthRedirectPort forces a fixed loopback port. Zero, the default, picks
	// a random one — correct for both Google desktop clients and Entra native
	// clients. Set it only where security software blocks binding arbitrary
	// ports, and register the same port with the provider.
	OAuthRedirectPort int

	// LogDir reports where the log files live. Injected rather than resolved
	// here so tests can point the export at a temporary directory.
	LogDir func() (string, error)

	// Retention bounds how much mail stays on disk. A zero policy keeps
	// everything.
	Retention model.RetentionPolicy

	// AttachmentDir reports where downloaded files are kept. Injected for the
	// same reason as LogDir: a test must be able to point it at a temporary
	// directory rather than the user's real one.
	AttachmentDir func() (string, error)

	// NotificationPreview puts the sender and subject in the new-mail
	// notification, rather than only a count.
	//
	// A setting because of where notifications end up. Windows shows them on
	// the lock screen unless told otherwise, and a client that argues for
	// privacy should not be the one deciding that a stranger standing at the
	// desk gets to read who wrote and about what. On by default: a
	// notification saying only "3 new messages" is one nobody can act on, and
	// Windows has its own switch for hiding content when locked.
	NotificationPreview bool

	// InvalidateBody drops a message from the reading pane's render cache.
	//
	// A closure for the same reason Emit is one: the cache is built from this
	// service, so the service cannot name its type without closing a cycle. It
	// is also why this is not an exported setter — every exported method on
	// the service becomes part of the window's API, and the window has no
	// business reaching into a cache.
	InvalidateBody func(messageID int64)
}

// LoadConfig reads config.json from the data directory, creating an empty one
// on first run so the user has a file to edit rather than a path to guess.
func LoadConfig(dir string) (Config, error) {
	path := filepath.Join(dir, configFileName)

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		blank, marshalErr := json.MarshalIndent(fileConfig{}, "", "  ")
		if marshalErr != nil {
			return Config{}, marshalErr
		}
		if writeErr := os.WriteFile(path, blank, 0o600); writeErr != nil {
			return Config{}, fmt.Errorf("app: creating %s: %w", configFileName, writeErr)
		}
		return Config{Retention: model.DefaultRetention}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("app: reading %s: %w", configFileName, err)
	}

	var fc fileConfig
	if err := json.Unmarshal(raw, &fc); err != nil {
		return Config{}, fmt.Errorf("app: %s is not valid JSON: %w", configFileName, err)
	}
	retention, err := fc.retention()
	if err != nil {
		return Config{}, err
	}

	return Config{
		GoogleClientID:      fc.GoogleClientID,
		MicrosoftClientID:   fc.MicrosoftClientID,
		OAuthRedirectPort:   fc.OAuthRedirectPort,
		Retention:           retention,
		NotificationPreview: fc.NotificationPreview == nil || *fc.NotificationPreview,
	}, nil
}

// retention resolves the two optional limits against the defaults.
//
// A negative value is rejected rather than clamped. Clamping it to zero would
// turn a typo into "keep everything" — the user would believe they had set a
// limit and find the database growing without bound, with nothing said.
func (fc fileConfig) retention() (model.RetentionPolicy, error) {
	policy := model.DefaultRetention

	if fc.RetentionDays != nil {
		if *fc.RetentionDays < 0 {
			return policy, fmt.Errorf(
				"app: retentionDays is %d; it cannot be negative (0 keeps everything)",
				*fc.RetentionDays)
		}
		policy.MaxAge = time.Duration(*fc.RetentionDays) * 24 * time.Hour
	}
	if fc.RetentionMaxMessages != nil {
		if *fc.RetentionMaxMessages < 0 {
			return policy, fmt.Errorf(
				"app: retentionMaxMessages is %d; it cannot be negative (0 keeps everything)",
				*fc.RetentionMaxMessages)
		}
		policy.MaxMessages = *fc.RetentionMaxMessages
	}
	return policy, nil
}
