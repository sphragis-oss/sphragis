// SPDX-License-Identifier: Apache-2.0

// Package vault stores a reversible token->original map, sealed at rest with
// AES-256-GCM. It is opt-in: without a key the gateway never records originals.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

var tokenRe = regexp.MustCompile(`\[([A-Z]+)_(\d+)\]`)

// Vault maps stable global tokens to their original values, persisted sealed.
type Vault struct {
	mu      sync.Mutex
	path    string
	gcm     cipher.AEAD
	fwd     map[string]string // "EMAIL_1" -> original
	rev     map[string]string // kind\x00value -> "EMAIL_1"
	counter map[string]int    // kind -> last assigned n
}

// Open loads and decrypts an existing vault, or starts an empty one at path.
func Open(path string, key [32]byte) (*Vault, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	v := &Vault{
		path: path, gcm: gcm,
		fwd: map[string]string{}, rev: map[string]string{}, counter: map[string]int{},
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return nil, err
	}
	if err := v.load(b); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *Vault) load(sealed []byte) error {
	ns := v.gcm.NonceSize()
	if len(sealed) < ns {
		return errors.New("vault file truncated")
	}
	plain, err := v.gcm.Open(nil, sealed[:ns], sealed[ns:], nil)
	if err != nil {
		return fmt.Errorf("decrypt vault (wrong key?): %w", err)
	}
	if err := json.Unmarshal(plain, &v.fwd); err != nil {
		return err
	}
	for tok, orig := range v.fwd {
		kind, n, ok := splitToken(tok)
		if !ok {
			continue
		}
		v.rev[kind+"\x00"+orig] = tok
		if n > v.counter[kind] {
			v.counter[kind] = n
		}
	}
	return nil
}

// Assign returns a stable global token (e.g. "EMAIL_1") for value, recording it.
func (v *Vault) Assign(kind, value string) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	rk := kind + "\x00" + value
	if t, ok := v.rev[rk]; ok {
		return t
	}
	v.counter[kind]++
	tok := kind + "_" + strconv.Itoa(v.counter[kind])
	v.fwd[tok] = value
	v.rev[rk] = tok
	_ = v.flush()
	return tok
}

// Reveal replaces [KIND_n] tokens with their originals where known.
func (v *Vault) Reveal(text string) string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return tokenRe.ReplaceAllStringFunc(text, func(m string) string {
		if orig, ok := v.fwd[strings.Trim(m, "[]")]; ok {
			return orig
		}
		return m
	})
}

// flush seals and atomically writes the map; caller holds the lock.
func (v *Vault) flush() error {
	plain, err := json.Marshal(v.fwd)
	if err != nil {
		return err
	}
	nonce := make([]byte, v.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := v.gcm.Seal(nonce, nonce, plain, nil)
	tmp := v.path + ".tmp"
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, v.path)
}

func splitToken(tok string) (kind string, n int, ok bool) {
	i := strings.LastIndexByte(tok, '_')
	if i <= 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(tok[i+1:])
	if err != nil {
		return "", 0, false
	}
	return tok[:i], n, true
}

// LoadKey resolves a 32-byte key from a base64 string or a key file (base64 or raw).
func LoadKey(b64, keyfile string) ([32]byte, bool, error) {
	var key [32]byte
	switch {
	case b64 != "":
		return decodeKey([]byte(b64))
	case keyfile != "":
		b, err := os.ReadFile(keyfile)
		if err != nil {
			return key, false, err
		}
		return decodeKey(b)
	default:
		return key, false, nil
	}
}

func decodeKey(b []byte) ([32]byte, bool, error) {
	var key [32]byte
	if raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b))); err == nil && len(raw) == 32 {
		copy(key[:], raw)
		return key, true, nil
	}
	if len(b) == 32 {
		copy(key[:], b)
		return key, true, nil
	}
	return key, false, errors.New("vault key must be 32 bytes (raw or base64)")
}

// DefaultPath is the sealed vault location under the sphragis home directory.
func DefaultPath(home string) string { return filepath.Join(home, "vault.bin") }
