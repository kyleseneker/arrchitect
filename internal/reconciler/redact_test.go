package reconciler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactRemovesRememberedSecrets(t *testing.T) {
	RememberSecret("hunter2-redact-test")
	RememberSecret("hunter2-redact-test-longer")
	RememberSecret("p@ss word/redact")
	RememberSecret("ab")

	assert.Equal(t, "key <redacted> and <redacted> rejected",
		Redact("key hunter2-redact-test-longer and hunter2-redact-test rejected"), "a secret containing another goes whole")
	assert.Equal(t, "GET /api?password=<redacted>", Redact("GET /api?password=p%40ss+word%2Fredact"), "URL-encoded")
	assert.Equal(t, "ab cd", Redact("ab cd"), "too short to tell from ordinary text")
}

func TestRememberSecretAsReplacesARotatedValue(t *testing.T) {
	RememberSecretAs("media/sonarr-credentials/api-key", "first-key-rotate-test")
	RememberSecretAs("media/sonarr-credentials/api-key", "second-key-rotate-test")

	assert.Equal(t, "first-key-rotate-test <redacted>", Redact("first-key-rotate-test second-key-rotate-test"),
		"only the current value of a Secret key is kept")
}
