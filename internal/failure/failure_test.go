package failure

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKindMarksAnError(t *testing.T) {
	assert.NoError(t, InvalidConfig.Wrap(nil))
	err := fmt.Errorf("connection: %w", InvalidConfig.Wrap(errors.New("bad URL")))
	assert.Equal(t, InvalidConfig, KindOf(err))
	assert.EqualError(t, err, "connection: bad URL")
	assert.Zero(t, KindOf(errors.New("bad URL")))
	assert.Zero(t, KindOf(nil))

	missing := SecretNotFound.Errorf("secret %q not found", "keys")
	assert.EqualError(t, missing, `secret "keys" not found`)
	assert.Equal(t, SecretNotFound, KindOf(missing))
}

// Joined failures read on one line in a status, and each stays findable.
func TestJoin(t *testing.T) {
	assert.NoError(t, Join())
	assert.NoError(t, Join(nil, nil))
	spec := InvalidConfig.Wrap(errors.New("b"))
	err := Join(errors.New("a"), nil, spec)
	assert.EqualError(t, err, "a; b")
	assert.Equal(t, InvalidConfig, KindOf(err))
	assert.ErrorIs(t, err, spec)
}

// Each failure joined into an error counts once, and a marked one stays whole
// with the failures it wraps.
func TestLeaves(t *testing.T) {
	refused := errors.New("refused")
	missing := SecretNotFound.Errorf("secret %q not found", "keys")
	held := InvalidConfig.Wrap(Join(errors.New("x"), errors.New("y")))

	leaves := Leaves(fmt.Errorf("pass: %w", Join(refused, fmt.Errorf("settings: %w", missing), held)))

	assert.Equal(t, []error{refused, missing, held}, leaves)
	assert.Equal(t, []error{refused}, Leaves(refused))
}
