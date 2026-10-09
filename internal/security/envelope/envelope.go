// Package envelope implements envelope encryption of secrets at rest.
//
// The server master key (configuration) wraps one random root key per user.
// Each user's root key derives, with HKDF-SHA256, a data key that seals the
// user's secrets and blobs, and a fingerprint key that names their blobs.
// Rotating the master key only rewraps the root keys: data never moves.
//
// Ciphertexts are AES-256-GCM with a random nonce, bound to their context
// (table, column, row, owner) through additional authenticated data, so a
// ciphertext copied to another row or user does not open.
package envelope

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
)

const (
	keySize       = 32
	formatVersion = 1
)

var (
	// ErrDecrypt means a ciphertext could not be opened: wrong key, wrong
	// context, tampered or truncated data. The cause is deliberately not told.
	ErrDecrypt = errors.New("envelope: cannot decrypt")
	// ErrUnknownKey means a wrapped key names a master key the keyring lacks.
	ErrUnknownKey = errors.New("envelope: unknown master key")
)

// Key is a 256-bit master key.
type Key struct {
	id   string
	aead cipher.AEAD
}

// GenerateKey returns a new random master key, base64-encoded.
func GenerateKey() string {
	return base64.StdEncoding.EncodeToString(randomBytes(keySize))
}

// ParseKey reads a base64-encoded 256-bit master key. Errors never quote it.
func ParseKey(encoded string) (Key, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(raw) != keySize {
		return Key{}, errors.New("a master key must be 32 bytes encoded in base64 (generate one with `magpie admin generate-key`)")
	}
	if bytes.Equal(raw, make([]byte, keySize)) {
		return Key{}, errors.New("a master key must not be all zeroes")
	}
	aead, err := newAEAD(raw)
	if err != nil {
		return Key{}, err
	}
	sum := sha256.Sum256(append([]byte("magpie master key id\x00"), raw...))
	return Key{id: hex.EncodeToString(sum[:8]), aead: aead}, nil
}

// ID identifies the key without revealing it. It is stored next to every key
// the master key wraps.
func (k Key) ID() string { return k.id }

// Keyring holds the current master key and previous ones, which are still
// needed to unwrap root keys until the rotation has rewrapped them.
type Keyring struct {
	current Key
	byID    map[string]Key
}

// NewKeyring returns a keyring that wraps with current and unwraps with any of
// the given keys.
func NewKeyring(current Key, previous ...Key) *Keyring {
	byID := map[string]Key{current.id: current}
	for _, k := range previous {
		byID[k.id] = k
	}
	return &Keyring{current: current, byID: byID}
}

// CurrentID is the ID of the key that Wrap uses.
func (kr *Keyring) CurrentID() string { return kr.current.id }

// Wrap encrypts a root key with the current master key, bound to aad.
func (kr *Keyring) Wrap(rootKey, aad []byte) (keyID string, wrapped []byte, err error) {
	return kr.current.id, seal(kr.current.aead, rootKey, aad), nil
}

// Unwrap decrypts a root key wrapped by the master key keyID.
func (kr *Keyring) Unwrap(keyID string, wrapped, aad []byte) ([]byte, error) {
	k, ok := kr.byID[keyID]
	if !ok {
		return nil, ErrUnknownKey
	}
	return open(k.aead, wrapped, aad)
}

// NewRootKey returns a new random per-user root key.
func NewRootKey() []byte { return randomBytes(keySize) }

// UserKey holds the keys derived from a user's root key.
type UserKey struct {
	data        cipher.AEAD
	fingerprint []byte
}

// DeriveUserKey derives a user's data and fingerprint keys from their root key.
func DeriveUserKey(rootKey []byte) (*UserKey, error) {
	if len(rootKey) != keySize {
		return nil, errors.New("envelope: a root key is 32 bytes")
	}
	dataKey, err := hkdf.Key(sha256.New, rootKey, nil, "magpie data key v1", keySize)
	if err != nil {
		return nil, fmt.Errorf("envelope: deriving the data key: %w", err)
	}
	fingerprintKey, err := hkdf.Key(sha256.New, rootKey, nil, "magpie fingerprint key v1", keySize)
	if err != nil {
		return nil, fmt.Errorf("envelope: deriving the fingerprint key: %w", err)
	}
	aead, err := newAEAD(dataKey)
	if err != nil {
		return nil, err
	}
	return &UserKey{data: aead, fingerprint: fingerprintKey}, nil
}

// Seal encrypts plaintext, bound to aad.
func (u *UserKey) Seal(plaintext, aad []byte) ([]byte, error) {
	return seal(u.data, plaintext, aad), nil
}

// Open decrypts what Seal produced with the same aad.
func (u *UserKey) Open(sealed, aad []byte) ([]byte, error) {
	return open(u.data, sealed, aad)
}

// Fingerprint returns the HMAC-SHA256 of content under the user's fingerprint
// key: equal for equal content of one user, unrelated across users, and
// useless to anyone who reads the database without the keys.
func (u *UserKey) Fingerprint(content io.Reader) ([]byte, error) {
	h := u.NewFingerprint()
	if _, err := io.Copy(h, content); err != nil {
		return nil, fmt.Errorf("envelope: reading content to fingerprint: %w", err)
	}
	return h.Sum(nil), nil
}

// NewFingerprint returns a hash that computes Fingerprint incrementally.
func (u *UserKey) NewFingerprint() hash.Hash {
	return hmac.New(sha256.New, u.fingerprint)
}

// AAD encodes context parts unambiguously (length-prefixed), for use as
// additional authenticated data.
func AAD(parts ...string) []byte {
	var b []byte
	for _, p := range parts {
		b = binary.BigEndian.AppendUint32(b, uint32(len(p)))
		b = append(b, p...)
	}
	return b
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("envelope: %w", err)
	}
	return cipher.NewGCM(block)
}

// seal returns version || nonce || ciphertext.
func seal(aead cipher.AEAD, plaintext, aad []byte) []byte {
	out := make([]byte, 1, 1+aead.NonceSize()+len(plaintext)+aead.Overhead())
	out[0] = formatVersion
	out = append(out, randomBytes(aead.NonceSize())...)
	return aead.Seal(out, out[1:], plaintext, aad)
}

func open(aead cipher.AEAD, sealed, aad []byte) ([]byte, error) {
	if len(sealed) < 1+aead.NonceSize()+aead.Overhead() || sealed[0] != formatVersion {
		return nil, ErrDecrypt
	}
	nonce := sealed[1 : 1+aead.NonceSize()]
	plaintext, err := aead.Open(nil, nonce, sealed[1+aead.NonceSize():], aad)
	if err != nil {
		return nil, ErrDecrypt
	}
	return plaintext, nil
}

// randomBytes panics only if the system's random source fails, which Go's
// crypto/rand treats as fatal anyway.
func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
