package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
)

const (
	secretsFileName = "secrets.enc"
	saltLen         = 16
	keyLen          = 32
)

// Argon2id parameters follow RFC 9106's second recommended option: 64 MiB of
// memory, 3 passes, 4 lanes. Comfortable for an interactive unlock, expensive
// enough that a stolen file is not worth brute-forcing.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 4
)

// fileStore keeps every secret in one AES-GCM sealed JSON blob. This is the
// fallback for systems with no usable OS keyring — chiefly Linux without a
// running Secret Service provider.
type fileStore struct {
	mu     sync.Mutex
	path   string
	master string
}

// secretsFile is the on-disk envelope. Storing the salt beside the ciphertext
// is standard and safe: a salt is not secret, it only has to be unique.
type secretsFile struct {
	Salt       []byte `json:"salt"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// NewFileStore opens or creates the encrypted secrets file in dir. When the
// file already exists the master password is verified immediately, so a wrong
// password fails here rather than surfacing later as a confusing decrypt error
// during a sync.
func NewFileStore(dir, masterPassword string) (SecretStore, error) {
	if masterPassword == "" {
		return nil, errors.New("auth: master password must not be empty")
	}

	s := &fileStore{
		path:   filepath.Join(dir, secretsFileName),
		master: masterPassword,
	}
	if _, err := os.Stat(s.path); err == nil {
		if _, err := s.load(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *fileStore) deriveKey(salt []byte) []byte {
	return argon2.IDKey([]byte(s.master), salt, argonTime, argonMemory, argonThreads, keyLen)
}

func (s *fileStore) load() (map[string]string, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("auth: read secrets file: %w", err)
	}

	var sf secretsFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		return nil, fmt.Errorf("auth: secrets file is corrupt: %w", err)
	}

	gcm, err := s.aead(sf.Salt)
	if err != nil {
		return nil, err
	}
	// GCM authenticates as it decrypts, so a wrong password fails here rather
	// than yielding plausible-looking garbage.
	plain, err := gcm.Open(nil, sf.Nonce, sf.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: cannot decrypt secrets file (wrong master password?): %w", err)
	}

	out := map[string]string{}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, fmt.Errorf("auth: decrypted secrets are malformed: %w", err)
	}
	return out, nil
}

func (s *fileStore) save(entries map[string]string) error {
	plain, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return fmt.Errorf("auth: generate salt: %w", err)
	}
	gcm, err := s.aead(salt)
	if err != nil {
		return err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("auth: generate nonce: %w", err)
	}

	blob, err := json.Marshal(secretsFile{
		Salt:       salt,
		Nonce:      nonce,
		Ciphertext: gcm.Seal(nil, nonce, plain, nil),
	})
	if err != nil {
		return err
	}

	// Write to a temp file and rename, so a crash mid-write cannot leave a
	// half-written secrets file — which would lock the user out of every
	// account at once.
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return fmt.Errorf("auth: write secrets file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("auth: replace secrets file: %w", err)
	}
	return nil
}

func (s *fileStore) aead(salt []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(s.deriveKey(salt))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *fileStore) Get(ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return "", err
	}
	v, ok := entries[ref]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *fileStore) Set(ref, secret string) error {
	if len(secret) > maxSecretBytes {
		return ErrSecretTooLong
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return err
	}
	entries[ref] = secret
	return s.save(entries)
}

func (s *fileStore) Delete(ref string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := s.load()
	if err != nil {
		return err
	}
	delete(entries, ref)
	return s.save(entries)
}
