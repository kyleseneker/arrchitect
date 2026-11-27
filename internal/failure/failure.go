// Package failure marks the kinds of failure a resource's status reports
// apart from an app refusing a write: a spec the operator can't act on, and
// several failures reported as one.
package failure

import (
	"errors"
	"strings"
)

// ConfigError is a spec the operator can't act on until it changes, such as a
// malformed URL.
type ConfigError struct{ err error }

func (e *ConfigError) Error() string { return e.err.Error() }
func (e *ConfigError) Unwrap() error { return e.err }

// InvalidConfig marks err as a problem with the spec, which no retry fixes.
func InvalidConfig(err error) error {
	if err == nil {
		return nil
	}
	return &ConfigError{err: err}
}

// IsConfigError reports whether err is a problem with the spec.
func IsConfigError(err error) bool {
	_, ok := errors.AsType[*ConfigError](err)
	return ok
}

// Join reports errs as one failure, nil when there are none. Unlike
// errors.Join it separates them with "; ", which reads on one line in a
// status message. errors.Is and errors.As still find each one.
func Join(errs ...error) error {
	var kept joinedErrors
	for _, err := range errs {
		if err != nil {
			kept = append(kept, err)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

type joinedErrors []error

func (e joinedErrors) Error() string {
	texts := make([]string, len(e))
	for i, err := range e {
		texts[i] = err.Error()
	}
	return strings.Join(texts, "; ")
}

func (e joinedErrors) Unwrap() []error { return e }
