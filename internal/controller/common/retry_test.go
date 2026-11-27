package common

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
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
		assert.GreaterOrEqual(t, got, engine.DefaultReconcileInterval)
		assert.LessOrEqual(t, got, engine.DefaultReconcileInterval+time.Duration(float64(engine.DefaultReconcileInterval)*jitter))
	}
}

func TestARecreatedResourceStartsWithoutTheOldOnesFailures(t *testing.T) {
	old := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "first"}
	RetryAfter(old, time.Minute)
	RetryAfter(old, time.Minute)

	recreated := &pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "second"}
	assert.Less(t, RetryAfter(recreated, time.Minute), 2*time.Minute, "a new UID starts at the base delay")

	forget(&pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media"})
	assert.Less(t, RetryAfter(old, time.Minute), 2*time.Minute, "a deleted resource's failures are dropped")
}

func TestAReplacedResourceLeavesNoEntryBehind(t *testing.T) {
	RetryAfter(&pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "first"}, time.Minute)
	RetryAfter(&pvrv1alpha1.Sonarr{Name: t.Name(), Namespace: "media", UID: "second"}, time.Minute)

	failures.Lock()
	defer failures.Unlock()
	var left []string
	for key := range failures.count {
		if strings.HasPrefix(key, "Sonarr/media/"+t.Name()+"/") {
			left = append(left, key)
		}
	}
	assert.Equal(t, []string{"Sonarr/media/" + t.Name() + "/second"}, left)
}
