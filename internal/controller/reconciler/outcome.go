package reconciler

import "github.com/kyleseneker/arrchitect/internal/failure"

// outcome is what a kind of failure means for a resource: the reason its
// status gives, and when it next reconciles.
type outcome struct {
	reason string
	// stalled marks a failure no retry fixes until something changes.
	stalled bool
	// backoff retries the failure through the controller's rate limiter,
	// rather than at the resource's interval.
	backoff bool
}

// outcomes is the failure policy. A failure that waits for the interval is
// often retried sooner: every controller watches the Secrets its resources
// name and the app resources their appRefs name.
var outcomes = map[failure.Kind]outcome{
	failure.Unreachable:       {reason: ReasonAppUnreachable, backoff: true},
	failure.Transient:         {reason: ReasonSyncFailed, backoff: true},
	failure.Unauthorized:      {reason: ReasonUnauthorized, stalled: true},
	failure.SecretNotFound:    {reason: ReasonSecretNotFound},
	failure.ReferenceNotFound: {reason: ReasonReferenceNotFound},
	failure.InvalidConfig:     {reason: ReasonInvalidConfig, stalled: true},
	failure.NameConflict:      {reason: ReasonNameConflict, stalled: true},
}

// refused is the outcome of an app refusing a write, or of any failure
// nobody marked.
var refused = outcome{reason: ReasonSyncFailed}

// outcomeOf is the outcome of a pass that failed with errs. Each failure
// counts once, including those joined into one error. Failures that are all of
// one kind take that kind's outcome, and any mix is refused.
func outcomeOf(errs []error) outcome {
	var kinds []failure.Kind
	for _, err := range errs {
		for _, leaf := range failure.Leaves(err) {
			kinds = append(kinds, failure.KindOf(leaf))
		}
	}
	if len(kinds) == 0 {
		return refused
	}
	for _, k := range kinds[1:] {
		if k != kinds[0] {
			return refused
		}
	}
	if out, ok := outcomes[kinds[0]]; ok {
		return out
	}
	return refused
}
