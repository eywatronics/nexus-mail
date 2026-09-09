// Package auth acquires and stores credentials. Secrets never reach the
// database or the config file: they live only in the OS keyring, or in an
// encrypted file when no keyring is available.
package auth

import (
	"errors"
	"os"
	"time"
)

var (
	// ErrNotFound reports that no secret is stored under the given ref.
	ErrNotFound = errors.New("auth: secret not found")

	// ErrSecretTooLong reports a secret the backend cannot hold. Windows
	// Credential Manager caps a credential blob at roughly 2.5 KB, so the
	// limit is applied uniformly: a secret that works on one platform then
	// works on all of them.
	ErrSecretTooLong = errors.New("auth: secret exceeds backend size limit")

	// ErrKeyringLocked reports that the OS credential store did not answer in
	// time, which almost always means it is locked and waiting for the user to
	// respond to a prompt.
	ErrKeyringLocked = errors.New("auth: OS keyring is locked or not responding")
)

// maxSecretBytes is the smallest limit across supported backends.
const maxSecretBytes = 2048

// keyringTimeout bounds every keyring call. A locked Secret Service or
// Keychain blocks until the user dismisses an OS prompt, and a background
// token refresh must never hang on that.
const keyringTimeout = 10 * time.Second

// masterPasswordEnv supplies the encrypted file store's key material. The UI
// prompt for it belongs to M2; until then this is the documented way to use
// the fallback on a system without a keyring.
const masterPasswordEnv = "NEXUSMAIL_MASTER_PASSWORD"

// SecretStore stores one secret per ref. A ref is an opaque key such as
// "account:user@example.com".
type SecretStore interface {
	Get(ref string) (string, error)
	Set(ref, secret string) error
	Delete(ref string) error
}

// Default returns the OS keyring when it is usable, otherwise an encrypted
// file store. Linux systems with no running Secret Service provider fall into
// the second case, which needs NEXUSMAIL_MASTER_PASSWORD to be set.
func Default(dir string) (SecretStore, error) {
	ks, err := NewKeyringStore()
	if err == nil {
		return ks, nil
	}

	master := os.Getenv(masterPasswordEnv)
	if master == "" {
		return nil, errors.New(
			"auth: no OS keyring available and " + masterPasswordEnv +
				" is unset; set it to enable the encrypted file store")
	}
	return NewFileStore(dir, master)
}
