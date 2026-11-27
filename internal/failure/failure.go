// Package failure marks an error with the kind of failure it is, where it
// happens, so a resource's status reason and its next pass follow from the
// error alone.
package failure

import (
	"errors"
	"fmt"
	"strings"
)

// Kind is what went wrong. The zero Kind marks nothing: an app refusing a
// write, or any failure nobody marked.
type Kind uint8

const (
	// Unreachable is an app that didn't answer, or answered that it's down.
	Unreachable Kind = iota + 1
	// Unauthorized is an app refusing the credentials.
	Unauthorized
	// SecretNotFound is a missing Secret, or a missing key in one.
	SecretNotFound
	// ReferenceNotFound is a missing resource the spec refers to.
	ReferenceNotFound
	// InvalidConfig is a spec, or a Secret it names, asking for what can't be
	// done.
	InvalidConfig
	// NameConflict is an older resource already configuring the item.
	NameConflict
	// Transient is a failure outside the app that a retry may fix, such as the
	// Kubernetes API timing out.
	Transient
)

// Wrap marks err with k, and keeps nil as nil.
func (k Kind) Wrap(err error) error {
	if err == nil {
		return nil
	}
	return &kindError{kind: k, err: err}
}

// Errorf formats an error marked with k.
func (k Kind) Errorf(format string, args ...any) error {
	return k.Wrap(fmt.Errorf(format, args...))
}

type kindError struct {
	kind Kind
	err  error
}

func (e *kindError) Error() string { return e.err.Error() }
func (e *kindError) Unwrap() error { return e.err }

// KindOf is the kind of the first marked error in err's chain, or zero.
func KindOf(err error) Kind {
	if k, ok := errors.AsType[*kindError](err); ok {
		return k.kind
	}
	return 0
}

// Leaves splits err into the failures joined into it, so each counts once. A
// marked error stays whole, with whatever it wraps.
func Leaves(err error) []error {
	for e := err; e != nil; {
		if _, ok := e.(*kindError); ok {
			return []error{e}
		}
		switch u := e.(type) {
		case interface{ Unwrap() []error }:
			var leaves []error
			for _, child := range u.Unwrap() {
				leaves = append(leaves, Leaves(child)...)
			}
			return leaves
		case interface{ Unwrap() error }:
			e = u.Unwrap()
		default:
			return []error{err}
		}
	}
	return []error{err}
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
