package reconciler

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
)

// maskedMinLen is the shortest run of asterisks treated as a mask. Real secrets
// are never all-asterisk, and the apps pad to at least this width.
const maskedMinLen = 4

// IsMasked reports whether a value fetched from an app is the placeholder these
// APIs substitute for secret fields on read. Sonarr, Radarr, Prowlarr and the
// rest return "********" for API keys and passwords, so the value the operator
// wrote back can never be read again to compare against.
func IsMasked(v any) bool {
	s, ok := v.(string)
	if !ok || len(s) < maskedMinLen {
		return false
	}
	// Autobrr substitutes the literal "<redacted>".
	return strings.Trim(s, "*") == "" || s == "<redacted>"
}

// secretDigests holds the hashes of the secret values the operator last wrote,
// by owning resource and then by item. A masked field cannot be compared
// against the app, so the only way to notice a rotated secret is to remember
// what was last sent. ForgetSecrets drops a deleted resource's entries.
//
// The store is process-scoped: after a restart the first pass rewrites each
// secret once and then goes quiet. That is a bounded, converging cost, unlike
// persisting a secret-derived hash into the status of every CRD.
var secretDigests = struct {
	sync.Mutex
	byOwner map[string]map[string]string
}{byOwner: map[string]map[string]string{}}

// recorded returns the digest last written for owner's key.
func recorded(owner, key string) (string, bool) {
	secretDigests.Lock()
	defer secretDigests.Unlock()
	digest, seen := secretDigests.byOwner[owner][key]
	return digest, seen
}

// SecretsDiffer reports whether masked secrets count as a difference. An
// empty digest means the item has no masked fields, so there is nothing to
// compare. Enforce mode rewrites them when nothing has been recorded yet, so
// the first pass after a restart sends them once. Observe mode never writes,
// so it cannot know a secret it has not seen written: only a recorded digest
// that no longer matches is a difference, or every masked credential would
// read as drift forever.
func SecretsDiffer(owner, key, digest string, observe bool) bool {
	if digest == "" {
		return false
	}
	prev, seen := recorded(owner, key)
	if !seen {
		return !observe
	}
	return prev != digest
}

// RecordSecrets stores the digest owner wrote for key.
func RecordSecrets(owner, key, digest string) {
	if digest == "" {
		return
	}
	secretDigests.Lock()
	defer secretDigests.Unlock()
	if secretDigests.byOwner[owner] == nil {
		secretDigests.byOwner[owner] = map[string]string{}
	}
	secretDigests.byOwner[owner][key] = digest
}

// ForgetSecrets drops the digests of every owner whose name starts with
// prefix, such as each incarnation of a resource that was deleted.
func ForgetSecrets(prefix string) {
	secretDigests.Lock()
	defer secretDigests.Unlock()
	for owner := range secretDigests.byOwner {
		if strings.HasPrefix(owner, prefix) {
			delete(secretDigests.byOwner, owner)
		}
	}
}

// digestOf hashes the desired values that landed on masked fields. Values are
// sorted so the digest does not depend on map iteration order.
func digestOf(values []string) string {
	if len(values) == 0 {
		return ""
	}
	sorted := append([]string(nil), values...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, v := range sorted {
		h.Write([]byte(v))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
