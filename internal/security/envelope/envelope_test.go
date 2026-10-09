package envelope_test

import (
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/veHRz/MagpieMail-Server/internal/security/envelope"
)

func masterKey(t *testing.T) envelope.Key {
	t.Helper()
	key, err := envelope.ParseKey(envelope.GenerateKey())
	require.NoError(t, err)
	return key
}

func userKey(t *testing.T) *envelope.UserKey {
	t.Helper()
	k, err := envelope.DeriveUserKey(envelope.NewRootKey())
	require.NoError(t, err)
	return k
}

func TestParseKey_AcceptsGeneratedKeys(t *testing.T) {
	encoded := envelope.GenerateKey()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	assert.Len(t, raw, 32)

	key, err := envelope.ParseKey(encoded)
	require.NoError(t, err)
	assert.Len(t, key.ID(), 16, "a short, stable hexadecimal ID")

	again, err := envelope.ParseKey(" " + encoded + "\n")
	require.NoError(t, err, "surrounding whitespace, as in a secret file, is ignored")
	assert.Equal(t, key.ID(), again.ID())
}

func TestParseKey_RejectsInvalidKeys(t *testing.T) {
	for name, value := range map[string]string{
		"empty":      "",
		"not base64": "not base64!",
		"too short":  base64.StdEncoding.EncodeToString(make([]byte, 16)),
		"too long":   base64.StdEncoding.EncodeToString(make([]byte, 64)),
		"all zeroes": base64.StdEncoding.EncodeToString(make([]byte, 32)),
	} {
		t.Run(name, func(t *testing.T) {
			_, err := envelope.ParseKey(value)
			require.Error(t, err)
			if value != "" {
				assert.NotContains(t, err.Error(), value, "the key never appears in an error")
			}
		})
	}
}

func TestGenerateKey_IsRandom(t *testing.T) {
	first, second := envelope.GenerateKey(), envelope.GenerateKey()
	assert.NotEqual(t, first, second)
}

func TestKeyring_WrapsWithTheCurrentKeyAndUnwrapsWithAnyKnownKey(t *testing.T) {
	old, current := masterKey(t), masterKey(t)
	root := envelope.NewRootKey()
	aad := envelope.AAD("user_keys", "user-1")

	oldRing := envelope.NewKeyring(old)
	oldID, wrapped, err := oldRing.Wrap(root, aad)
	require.NoError(t, err)
	assert.Equal(t, old.ID(), oldID)
	assert.NotContains(t, string(wrapped), string(root))

	ring := envelope.NewKeyring(current, old)
	assert.Equal(t, current.ID(), ring.CurrentID())
	unwrapped, err := ring.Unwrap(oldID, wrapped, aad)
	require.NoError(t, err, "a previous key still opens what it wrapped")
	assert.Equal(t, root, unwrapped)

	newID, rewrapped, err := ring.Wrap(unwrapped, aad)
	require.NoError(t, err)
	assert.Equal(t, current.ID(), newID)
	unwrapped, err = envelope.NewKeyring(current).Unwrap(newID, rewrapped, aad)
	require.NoError(t, err, "once rewrapped, the old key is no longer needed")
	assert.Equal(t, root, unwrapped)
}

func TestKeyring_RefusesUnknownKeysAndForeignContexts(t *testing.T) {
	ring := envelope.NewKeyring(masterKey(t))
	id, wrapped, err := ring.Wrap(envelope.NewRootKey(), envelope.AAD("user_keys", "user-1"))
	require.NoError(t, err)

	_, err = envelope.NewKeyring(masterKey(t)).Unwrap(id, wrapped, envelope.AAD("user_keys", "user-1"))
	require.ErrorIs(t, err, envelope.ErrUnknownKey)

	_, err = ring.Unwrap(id, wrapped, envelope.AAD("user_keys", "user-2"))
	require.ErrorIs(t, err, envelope.ErrDecrypt, "a wrapped key moved to another user does not open")
}

func TestUserKey_SealsAndOpens(t *testing.T) {
	k := userKey(t)
	aad := envelope.AAD("accounts", "credentials", "account-1")
	plaintext := []byte("refresh-token-do-not-leak")

	sealed, err := k.Seal(plaintext, aad)
	require.NoError(t, err)
	assert.False(t, bytes.Contains(sealed, plaintext))

	opened, err := k.Open(sealed, aad)
	require.NoError(t, err)
	assert.Equal(t, plaintext, opened)

	again, err := k.Seal(plaintext, aad)
	require.NoError(t, err)
	assert.NotEqual(t, sealed, again, "a fresh nonce for every seal")
}

func TestUserKey_DetectsTamperingAndMisuse(t *testing.T) {
	k := userKey(t)
	aad := envelope.AAD("accounts", "credentials", "account-1")
	sealed, err := k.Seal([]byte("secret"), aad)
	require.NoError(t, err)

	tampered := bytes.Clone(sealed)
	tampered[len(tampered)-1] ^= 1
	_, err = k.Open(tampered, aad)
	require.ErrorIs(t, err, envelope.ErrDecrypt)

	_, err = k.Open(sealed, envelope.AAD("accounts", "credentials", "account-2"))
	require.ErrorIs(t, err, envelope.ErrDecrypt, "a ciphertext copied to another row does not open")

	_, err = userKey(t).Open(sealed, aad)
	require.ErrorIs(t, err, envelope.ErrDecrypt, "another user's key does not open it")

	_, err = k.Open([]byte{9, 1, 2}, aad)
	require.ErrorIs(t, err, envelope.ErrDecrypt, "unknown format version or truncated data")
}

func TestUserKey_IsDerivedDeterministicallyFromTheRootKey(t *testing.T) {
	root := envelope.NewRootKey()
	a, err := envelope.DeriveUserKey(root)
	require.NoError(t, err)
	b, err := envelope.DeriveUserKey(root)
	require.NoError(t, err)

	sealed, err := a.Seal([]byte("x"), nil)
	require.NoError(t, err)
	opened, err := b.Open(sealed, nil)
	require.NoError(t, err)
	assert.Equal(t, []byte("x"), opened)

	_, err = envelope.DeriveUserKey([]byte("short"))
	require.Error(t, err)
}

func TestUserKey_FingerprintsAreKeyedPerUser(t *testing.T) {
	alice, bob := userKey(t), userKey(t)
	content := "the same invoice"

	fp := func(k *envelope.UserKey) []byte {
		sum, err := k.Fingerprint(strings.NewReader(content))
		require.NoError(t, err)
		return sum
	}

	first, second := fp(alice), fp(alice)
	assert.Equal(t, first, second, "stable for one user")
	assert.Len(t, fp(alice), 32)
	assert.NotEqual(t, fp(alice), fp(bob), "two users never share a fingerprint")

	sum, err := alice.Fingerprint(io.MultiReader(strings.NewReader("the same "), strings.NewReader("invoice")))
	require.NoError(t, err)
	assert.Equal(t, fp(alice), sum, "computed over the stream")
}

func TestAAD_IsUnambiguous(t *testing.T) {
	assert.NotEqual(t, envelope.AAD("ab", "c"), envelope.AAD("a", "bc"))
	first, second := envelope.AAD("a", "b"), envelope.AAD("a", "b")
	assert.Equal(t, first, second)
}
