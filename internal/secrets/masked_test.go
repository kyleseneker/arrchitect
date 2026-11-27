package secrets

import (
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsMasked(t *testing.T) {
	assert.True(t, IsMasked("********"))
	assert.True(t, IsMasked("****"))
	assert.False(t, IsMasked("***"))
	assert.False(t, IsMasked(""))
	assert.False(t, IsMasked("hunter2"))
	assert.False(t, IsMasked("abc****"))
	assert.False(t, IsMasked(nil))
	assert.False(t, IsMasked(8))
}

// A rotated secret is invisible in the fetched state, so the only signal is a
// digest that differs from the one last written.
func TestSecretsDifferTracksRotation(t *testing.T) {
	owner := freshOwner(t)
	const key = "notifications/Jellyfin"

	first := Digest([]string{"key-v1"})
	require.NotEmpty(t, first)

	assert.True(t, SecretsDiffer(owner, key, first, false), "never written, so it must write")
	RecordSecrets(owner, key, first)
	assert.False(t, SecretsDiffer(owner, key, first, false), "unchanged secret must stay quiet")

	rotated := Digest([]string{"key-v2"})
	assert.True(t, SecretsDiffer(owner, key, rotated, false), "rotated secret must write")
	RecordSecrets(owner, key, rotated)
	assert.False(t, SecretsDiffer(owner, key, rotated, false))
}

// freshOwner is an owner no other test or earlier run under -count has
// recorded for, since the store lives for the process as it does in the
// operator. Its records are dropped when the test ends.
func freshOwner(t *testing.T) string {
	t.Helper()
	owner := "Test/" + t.Name() + "/" + rand.Text()
	t.Cleanup(func() { ForgetSecrets(owner) })
	return owner
}

func TestSecretsDifferIgnoresEmptyDigest(t *testing.T) {
	assert.False(t, SecretsDiffer(freshOwner(t), "type/name", "", false))
}

// Observe mode never writes, so after a restart it has no record of what a
// masked secret holds. Counting that as drift would report every masked
// credential as differing forever.
func TestSecretsDifferOnlyFromARecordUnderObserve(t *testing.T) {
	owner := freshOwner(t)
	const key = "downloadClients/qbit"
	written := Digest([]string{"pw-v1"})

	assert.False(t, SecretsDiffer(owner, key, written, true), "nothing recorded: unverifiable, not drift")
	assert.True(t, SecretsDiffer(owner, key, written, false), "enforce still sends it once")

	RecordSecrets(owner, key, written)
	assert.False(t, SecretsDiffer(owner, key, written, true))
	assert.True(t, SecretsDiffer(owner, key, Digest([]string{"pw-v2"}), true), "a rotation away from what was written is drift")
}

// A resource's records are its own: another resource writing the same item
// doesn't silence it.
func TestSecretsAreRecordedPerOwner(t *testing.T) {
	a, b := freshOwner(t), freshOwner(t)
	digest := Digest([]string{"pw"})
	RecordSecrets(a, "downloadClients/qbit", digest)
	assert.True(t, SecretsDiffer(b, "downloadClients/qbit", digest, false))
}

// Deleting a resource drops what every incarnation of it recorded, and
// nothing another resource recorded.
func TestForgetSecretsDropsEveryOwnerUnderThePrefix(t *testing.T) {
	prefix := "Test/" + rand.Text() + "/"
	first, second, other := prefix+"uid-1", prefix+"uid-2", freshOwner(t)
	digest := Digest([]string{"pw"})
	for _, owner := range []string{first, second, other} {
		RecordSecrets(owner, "settings/host", digest)
	}

	ForgetSecrets(prefix)

	assert.True(t, SecretsDiffer(first, "settings/host", digest, false))
	assert.True(t, SecretsDiffer(second, "settings/host", digest, false))
	assert.False(t, SecretsDiffer(other, "settings/host", digest, false))
}

func TestDigestOfIsOrderIndependent(t *testing.T) {
	assert.Equal(t, Digest([]string{"a", "b"}), Digest([]string{"b", "a"}))
	assert.NotEqual(t, Digest([]string{"a"}), Digest([]string{"b"}))
	assert.Empty(t, Digest(nil))
}

func TestIsMaskedRecognizesAutobrrRedaction(t *testing.T) {
	assert.True(t, IsMasked("<redacted>"))
	assert.False(t, IsMasked("redacted"))
}
