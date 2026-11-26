package common

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
)

func TestRetryAfterBacksOffUntilTheAppAnswers(t *testing.T) {
	obj := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media"}
	within := func(want time.Duration, got time.Duration, msg string) {
		t.Helper()
		assert.GreaterOrEqual(t, got, want, msg)
		assert.LessOrEqual(t, got, want+time.Duration(float64(want)*jitter), msg)
	}

	within(time.Minute, RetryAfter(obj, time.Minute), "the first retry waits the base delay")
	within(2*time.Minute, RetryAfter(obj, time.Minute), "each failure in a row doubles it")
	within(4*time.Minute, RetryAfter(obj, time.Minute), "each failure in a row doubles it")
	within(8*time.Minute, RetryAfter(obj, time.Minute), "each failure in a row doubles it")
	for range 20 {
		within(maxRetryDelay, RetryAfter(obj, time.Minute), "the wait never passes the cap")
	}

	Reached(obj)
	within(time.Minute, RetryAfter(obj, time.Minute), "once the app answers, backoff starts over")
	assert.Zero(t, RetryAfter(obj, 0), "a spec problem waits for the spec to change")
}

func TestRetriesAreCountedPerResource(t *testing.T) {
	sonarr := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media"}
	radarr := &pvrv1alpha1.Radarr{Name: t.Name(), Namespace: "media"}
	RetryAfter(sonarr, time.Minute)
	RetryAfter(sonarr, time.Minute)

	got := RetryAfter(radarr, time.Minute)
	assert.Less(t, got, 2*time.Minute, "another kind with the same name starts at the base delay")
}

func TestNextSyncStretchesTheIntervalALittle(t *testing.T) {
	for range 50 {
		got := NextSync(nil)
		assert.GreaterOrEqual(t, got, DefaultReconcileInterval)
		assert.LessOrEqual(t, got, DefaultReconcileInterval+time.Duration(float64(DefaultReconcileInterval)*jitter))
	}
}
