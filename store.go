package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Binding maps a Feishu user to their intranet account. Only the PocketBase token is stored,
// never the password. The on-disk format matches the Node version byte for byte:
// 12-byte IV | 16-byte GCM tag | ciphertext of JSON {open_id: {email, token}}.
type Binding struct {
	Email string `json:"email"`
	Token string `json:"token"`
}

type Store struct {
	mu   sync.Mutex
	path string
	aead cipher.AEAD
	m    map[string]*Binding
}

func loadKey() (cipher.AEAD, error) {
	key, err := hex.DecodeString(os.Getenv("BIND_KEY"))
	if err != nil || len(key) != 32 {
		return nil, errors.New("BIND_KEY must be 64 hex chars")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func OpenStore(dir string) (*Store, error) {
	aead, err := loadKey()
	if err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "bindings.enc"), aead: aead, m: map[string]*Binding{}}
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) < 28 {
		return nil, errors.New("bindings.enc is corrupt")
	}
	// Go's GCM expects ciphertext||tag; the file stores the tag before the ciphertext.
	sealed := append(append([]byte{}, raw[28:]...), raw[12:28]...)
	plain, err := aead.Open(nil, raw[:12], sealed, nil)
	if err != nil {
		return nil, errors.New("cannot decrypt bindings.enc (wrong BIND_KEY?)")
	}
	if err := json.Unmarshal(plain, &s.m); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Get(openID string) (Binding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.m[openID]
	if !ok {
		return Binding{}, false
	}
	return *b, true
}

func (s *Store) FindByEmail(email string) (Binding, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, b := range s.m {
		if b.Email == email {
			return *b, true
		}
	}
	return Binding{}, false
}

func (s *Store) Emails() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, b := range s.m {
		out = append(out, b.Email)
	}
	return out
}

func (s *Store) Set(openID string, b Binding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[openID] = &b
	return s.saveLocked()
}

func (s *Store) Delete(openID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, openID)
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	plain, err := json.Marshal(s.m)
	if err != nil {
		return err
	}
	iv := make([]byte, 12)
	if _, err := rand.Read(iv); err != nil {
		return err
	}
	sealed := s.aead.Seal(nil, iv, plain, nil)
	ct, tag := sealed[:len(sealed)-16], sealed[len(sealed)-16:]
	out := append(append(append([]byte{}, iv...), tag...), ct...)
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
