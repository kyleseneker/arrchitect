package reconciler

import (
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
func TestSecretsChangedSinceTracksRotation(t *testing.T) {
	const key = "sonarr|notifications|Jellyfin"
	forgetSecretsAfter(t, key)

	first := digestOf([]string{"key-v1"})
	require.NotEmpty(t, first)

	assert.True(t, SecretsChangedSince(key, first), "never written, so it must write")
	RecordSecrets(key, first)
	assert.False(t, SecretsChangedSince(key, first), "unchanged secret must stay quiet")

	rotated := digestOf([]string{"key-v2"})
	assert.True(t, SecretsChangedSince(key, rotated), "rotated secret must write")
	RecordSecrets(key, rotated)
	assert.False(t, SecretsChangedSince(key, rotated))
}

// forgetSecretsAfter drops what a test recorded for key. The store lives for the
// process, as it does in the operator, so a record left behind would answer the
// same test's next run under -count.
func forgetSecretsAfter(t *testing.T, key string) {
	t.Helper()
	t.Cleanup(func() {
		secretDigest.Lock()
		defer secretDigest.Unlock()
		delete(secretDigest.hashes, key)
	})
}

func TestSecretsChangedSinceIgnoresEmptyDigest(t *testing.T) {
	assert.False(t, SecretsChangedSince("app|type|name", ""))
}

// Observe mode never writes, so after a restart it has no record of what a
// masked secret holds. Counting that as drift would report every masked
// credential as differing forever.
func TestSecretsDifferOnlyFromARecordUnderObserve(t *testing.T) {
	const key = "radarr|downloadClients|qbit"
	forgetSecretsAfter(t, key)
	written := digestOf([]string{"pw-v1"})

	assert.False(t, SecretsDiffer(key, written, true), "nothing recorded: unverifiable, not drift")
	assert.True(t, SecretsDiffer(key, written, false), "enforce still sends it once")

	RecordSecrets(key, written)
	assert.False(t, SecretsDiffer(key, written, true))
	assert.True(t, SecretsDiffer(key, digestOf([]string{"pw-v2"}), true), "a rotation away from what was written is drift")
}

func TestDigestOfIsOrderIndependent(t *testing.T) {
	assert.Equal(t, digestOf([]string{"a", "b"}), digestOf([]string{"b", "a"}))
	assert.NotEqual(t, digestOf([]string{"a"}), digestOf([]string{"b"}))
	assert.Empty(t, digestOf(nil))
}

func TestIsMaskedRecognizesAutobrrRedaction(t *testing.T) {
	assert.True(t, IsMasked("<redacted>"))
	assert.False(t, IsMasked("redacted"))
}
