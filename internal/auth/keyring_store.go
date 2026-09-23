package auth

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

const keyringService = "nexus-mail"

// probeRef is written and deleted during construction to find out whether a
// usable keyring backend exists. Probing beats inspecting the environment,
// which cannot tell whether a Secret Service provider is actually running —
// only whether something claims it should be.
const probeRef = "__nexusmail_probe__"

type keyringStore struct{}

// NewKeyringStore returns a store backed by the OS keyring, or an error when
// no keyring is reachable. The probe is itself timeout-bounded: on a locked
// keyring even this call blocks on an OS prompt.
func NewKeyringStore() (SecretStore, error) {
	err := withTimeoutVoid(func() error {
		if err := keyring.Set(keyringService, probeRef, "probe"); err != nil {
			return fmt.Errorf("not writable: %w", err)
		}
		return keyring.Delete(keyringService, probeRef)
	})
	if err != nil {
		return nil, fmt.Errorf("auth: OS keyring unavailable: %w", err)
	}
	return &keyringStore{}, nil
}

func (k *keyringStore) Get(ref string) (string, error) {
	return withTimeout(func() (string, error) {
		v, err := keyring.Get(keyringService, ref)
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		if err != nil {
			return "", fmt.Errorf("auth: read %q from keyring: %w", ref, err)
		}
		return v, nil
	})
}

func (k *keyringStore) Set(ref, secret string) error {
	if len(secret) > maxSecretBytes {
		return ErrSecretTooLong
	}
	return withTimeoutVoid(func() error {
		if err := keyring.Set(keyringService, ref, secret); err != nil {
			return fmt.Errorf("auth: write %q to keyring: %w", ref, err)
		}
		return nil
	})
}

// Delete is idempotent: removing an account whose secret is already gone is a
// success, not a failure.
func (k *keyringStore) Delete(ref string) error {
	return withTimeoutVoid(func() error {
		err := keyring.Delete(keyringService, ref)
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("auth: delete %q from keyring: %w", ref, err)
		}
		return nil
	})
}
