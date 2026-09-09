package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestFileStoreRoundTrip(t *testing.T) {
	s, err := NewFileStore(t.TempDir(), "correct horse battery staple")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}

	if err := s.Set("account:a@example.com", "refresh-token-value"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}
	got, err := s.Get("account:a@example.com")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "refresh-token-value" {
		t.Errorf("Get() = %q, want refresh-token-value", got)
	}

	if err := s.Delete("account:a@example.com"); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}
	if _, err := s.Get("account:a@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() after Delete returned %v, want ErrNotFound", err)
	}
}

func TestFileStoreHoldsSeveralSecrets(t *testing.T) {
	s, err := NewFileStore(t.TempDir(), "master")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}

	// Multiple accounts share one encrypted file, so writing one must not
	// clobber the others.
	for _, ref := range []string{"a", "b", "c"} {
		if err := s.Set("account:"+ref, "secret-"+ref); err != nil {
			t.Fatalf("Set(%s) error: %v", ref, err)
		}
	}
	for _, ref := range []string{"a", "b", "c"} {
		got, err := s.Get("account:" + ref)
		if err != nil {
			t.Fatalf("Get(%s) error: %v", ref, err)
		}
		if got != "secret-"+ref {
			t.Errorf("Get(%s) = %q, want secret-%s", ref, got, ref)
		}
	}
}

func TestFileStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()

	s1, err := NewFileStore(dir, "master-pw")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := s1.Set("k", "v"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	s2, err := NewFileStore(dir, "master-pw")
	if err != nil {
		t.Fatalf("second NewFileStore() error: %v", err)
	}
	got, err := s2.Get("k")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "v" {
		t.Errorf("Get() = %q, want v", got)
	}
}

func TestFileStoreRejectsWrongMasterPassword(t *testing.T) {
	dir := t.TempDir()

	s1, err := NewFileStore(dir, "right-password")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := s1.Set("k", "v"); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	// Failing at construction is the preferred behaviour, but failing on the
	// first read is acceptable. What must never happen is returning garbage.
	s2, err := NewFileStore(dir, "wrong-password")
	if err != nil {
		return
	}
	if got, err := s2.Get("k"); err == nil {
		t.Errorf("Get() with the wrong master password returned %q and no error; AES-GCM authentication is not being checked", got)
	}
}

func TestFileStoreCiphertextDoesNotLeakPlaintext(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileStore(dir, "master-pw")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}

	const secret = "SUPER-SECRET-REFRESH-TOKEN"
	if err := s.Set("k", secret); err != nil {
		t.Fatalf("Set() error: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, secretsFileName))
	if err != nil {
		t.Fatalf("read secrets file: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Error("secrets file contains the plaintext secret")
	}
	if strings.Contains(string(raw), "master-pw") {
		t.Error("secrets file contains the master password")
	}
}

func TestFileStoreRejectsOversizedSecret(t *testing.T) {
	s, err := NewFileStore(t.TempDir(), "master-pw")
	if err != nil {
		t.Fatalf("NewFileStore() error: %v", err)
	}
	if err := s.Set("k", strings.Repeat("x", maxSecretBytes+1)); !errors.Is(err, ErrSecretTooLong) {
		t.Errorf("Set() with an oversized secret returned %v, want ErrSecretTooLong", err)
	}
}

func TestFileStoreRejectsEmptyMasterPassword(t *testing.T) {
	if _, err := NewFileStore(t.TempDir(), ""); err == nil {
		t.Error("NewFileStore() accepted an empty master password")
	}
}

func TestDefaultChoosesAnAvailableBackend(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(masterPasswordEnv, "ci-master-password")

	s, err := Default(dir)
	if err != nil {
		t.Fatalf("Default() error: %v", err)
	}
	if err := s.Set("probe", "value"); err != nil {
		t.Fatalf("Set() on the chosen backend failed: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Delete("probe"); err != nil {
			t.Errorf("Delete() error: %v", err)
		}
	})

	got, err := s.Get("probe")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != "value" {
		t.Errorf("Get() = %q, want value", got)
	}
}

func TestDefaultExplainsItselfWhenNothingIsAvailable(t *testing.T) {
	if _, err := NewKeyringStore(); err == nil {
		t.Skip("an OS keyring is available on this machine, so this path cannot be exercised here")
	}
	t.Setenv(masterPasswordEnv, "")

	_, err := Default(t.TempDir())
	if err == nil {
		t.Fatal("Default() succeeded with no keyring and no master password")
	}
	// An error the user cannot act on is barely better than a crash.
	if !strings.Contains(err.Error(), masterPasswordEnv) {
		t.Errorf("error message %q does not tell the user how to fix it", err)
	}
}

// A locked keyring blocks until the user dismisses an OS prompt. A background
// token refresh that waits on that would hang the app, so every call is
// bounded. synctest fakes the clock, so this asserts a ten-second timeout
// without taking ten seconds.
func TestWithTimeoutGivesUpOnABlockedKeyring(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocked := make(chan struct{})
		t.Cleanup(func() { close(blocked) })

		if _, err := withTimeout(func() (string, error) {
			<-blocked
			return "", nil
		}); !errors.Is(err, ErrKeyringLocked) {
			t.Errorf("withTimeout() error = %v, want ErrKeyringLocked", err)
		}
	})
}

func TestWithTimeoutFailsFastWhileAnotherCallIsStuck(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		blocked := make(chan struct{})
		t.Cleanup(func() { close(blocked) })

		go func() {
			_, _ = withTimeout(func() (string, error) {
				<-blocked
				return "", nil
			})
		}()
		// Let the first call register itself and time out.
		time.Sleep(keyringTimeout + time.Second)

		start := time.Now()
		if _, err := withTimeout(func() (string, error) { return "x", nil }); !errors.Is(err, ErrKeyringLocked) {
			t.Errorf("second call error = %v, want ErrKeyringLocked", err)
		}
		// The point of failing fast: no second ten-second wait, and no second
		// leaked goroutine on every sync cycle.
		if elapsed := time.Since(start); elapsed >= keyringTimeout {
			t.Errorf("second call waited %v; it should fail immediately", elapsed)
		}
	})
}

func TestWithTimeoutPassesThroughAFastCall(t *testing.T) {
	got, err := withTimeout(func() (string, error) { return "value", nil })
	if err != nil {
		t.Fatalf("withTimeout() error: %v", err)
	}
	if got != "value" {
		t.Errorf("withTimeout() = %q, want value", got)
	}

	// The gate must be open again afterwards, or one slow-but-successful call
	// would wedge every later one.
	if _, err := withTimeout(func() (string, error) { return "again", nil }); err != nil {
		t.Errorf("second call after a successful one returned %v", err)
	}
}

func TestWithTimeoutPropagatesTheCallsError(t *testing.T) {
	sentinel := errors.New("backend exploded")
	if _, err := withTimeout(func() (string, error) { return "", sentinel }); !errors.Is(err, sentinel) {
		t.Errorf("withTimeout() error = %v, want the call's own error", err)
	}
}
