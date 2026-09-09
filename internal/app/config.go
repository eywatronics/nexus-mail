package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const configFileName = "config.json"

type fileConfig struct {
	GoogleClientID    string `json:"googleClientId"`
	MicrosoftClientID string `json:"microsoftClientId"`
	OAuthRedirectPort int    `json:"oauthRedirectPort"`
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
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("app: reading %s: %w", configFileName, err)
	}

	var fc fileConfig
	if err := json.Unmarshal(raw, &fc); err != nil {
		return Config{}, fmt.Errorf("app: %s is not valid JSON: %w", configFileName, err)
	}
	return Config{
		GoogleClientID:    fc.GoogleClientID,
		MicrosoftClientID: fc.MicrosoftClientID,
		OAuthRedirectPort: fc.OAuthRedirectPort,
	}, nil
}
