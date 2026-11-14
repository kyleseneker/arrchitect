package reconciler

import (
	"cmp"
	"net/url"
	"slices"
	"strings"
	"sync"
)

// redactions holds the secret values this process has read, so text an app
// echoes them in can be cleaned before it reaches a status, an event or a log.
var redactions = struct {
	sync.RWMutex
	values map[string]struct{}
}{values: map[string]struct{}{}}

// minRedactLen leaves out values too short to tell from ordinary words.
const minRedactLen = 4

// RememberSecret marks value as a secret Redact removes.
func RememberSecret(value string) {
	if len(value) < minRedactLen {
		return
	}
	redactions.Lock()
	defer redactions.Unlock()
	redactions.values[value] = struct{}{}
}

// Redact replaces every remembered secret in text, as written or URL-encoded,
// with <redacted>. Longer secrets go first, so one that contains another is
// removed whole.
func Redact(text string) string {
	redactions.RLock()
	values := make([]string, 0, len(redactions.values))
	for v := range redactions.values {
		values = append(values, v)
	}
	redactions.RUnlock()
	slices.SortFunc(values, func(a, b string) int { return cmp.Compare(len(b), len(a)) })
	for _, v := range values {
		text = strings.ReplaceAll(text, v, "<redacted>")
		if escaped := url.QueryEscape(v); escaped != v {
			text = strings.ReplaceAll(text, escaped, "<redacted>")
		}
	}
	return text
}
