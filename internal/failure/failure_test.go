package failure

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInvalidConfig(t *testing.T) {
	assert.NoError(t, InvalidConfig(nil))
	err := fmt.Errorf("connection: %w", InvalidConfig(errors.New("bad URL")))
	assert.True(t, IsConfigError(err))
	assert.EqualError(t, err, "connection: bad URL")
	assert.False(t, IsConfigError(errors.New("bad URL")))
}

// Joined failures read on one line in a status, and each stays findable.
func TestJoin(t *testing.T) {
	assert.NoError(t, Join())
	assert.NoError(t, Join(nil, nil))
	spec := InvalidConfig(errors.New("b"))
	err := Join(errors.New("a"), nil, spec)
	assert.EqualError(t, err, "a; b")
	assert.True(t, IsConfigError(err))
	assert.ErrorIs(t, err, spec)
}
