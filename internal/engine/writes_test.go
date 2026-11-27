package engine

import (
	"errors"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kyleseneker/arrchitect/internal/metrics"
)

func TestWritesApplyEnforceWritesAndCountsTheCorrection(t *testing.T) {
	corrected := metrics.DriftCorrectedTotal.WithLabelValues("writes-enforce", "settings", "main")
	before := testutil.ToFloat64(corrected)

	w := &Writes{App: "writes-enforce"}
	wrote := false
	require.NoError(t, w.Apply("settings", "main", func() error { wrote = true; return nil }))

	assert.True(t, wrote, "enforce mode must send the write")
	assert.Equal(t, before+1, testutil.ToFloat64(corrected), "drift_corrected_total")
	assert.Empty(t, w.Drift(), "enforce mode records no drift")
}

func TestWritesApplyEnforceReturnsTheWriteError(t *testing.T) {
	w := &Writes{}
	err := w.Apply("settings", "main", func() error { return errors.New("rejected") })
	require.EqualError(t, err, "rejected")
}

func TestWritesApplyObserveRecordsInsteadOfWriting(t *testing.T) {
	observed := metrics.DriftObservedTotal.WithLabelValues("writes-observe", "settings", "main")
	corrected := metrics.DriftCorrectedTotal.WithLabelValues("writes-observe", "settings", "main")
	beforeObserved, beforeCorrected := testutil.ToFloat64(observed), testutil.ToFloat64(corrected)

	w := &Writes{App: "writes-observe", Observe: true}
	require.NoError(t, w.Apply("settings", "main", func() error {
		require.Fail(t, "observe mode must not write")
		return nil
	}))

	assert.Equal(t, []string{"settings(main)"}, w.Drift())
	assert.Equal(t, beforeObserved+1, testutil.ToFloat64(observed), "drift_observed_total")
	assert.Equal(t, beforeCorrected, testutil.ToFloat64(corrected), "drift_corrected_total moved in observe mode")
}

func TestWritesRecordDrift(t *testing.T) {
	observed := metrics.DriftObservedTotal.WithLabelValues("writes-record", "action", "a")
	before := testutil.ToFloat64(observed)

	w := &Writes{App: "writes-record", Observe: true}
	w.RecordDrift("action", "a")
	w.RecordDrift("action", "b")

	assert.Equal(t, []string{"action(a)", "action(b)"}, w.Drift())
	assert.Equal(t, before+1, testutil.ToFloat64(observed), "drift_observed_total for action/a")
}

func TestWritesPruneObserveRecordsWhatItWouldDelete(t *testing.T) {
	owned := NewOwnership("writes-prune", map[string][]string{"feeds": {"old", "kept"}})
	w := &Writes{App: "writes-prune", Observe: true}

	require.NoError(t, w.Prune(owned, "feeds", []string{"kept"}, []string{"old", "kept", "hand-made"}, func(string) error {
		require.Fail(t, "observe mode must not delete")
		return nil
	}))

	assert.Equal(t, []string{"feeds(old)"}, w.Drift())
	assert.Equal(t, map[string][]string{"feeds": {"old", "kept"}}, owned.Managed(), "observe mode changed ownership")
}

func TestWritesPruneEnforceDeletes(t *testing.T) {
	owned := NewOwnership("writes-prune", map[string][]string{"feeds": {"old"}})
	var removed []string
	require.NoError(t, (&Writes{}).Prune(owned, "feeds", []string{"kept"}, []string{"old", "kept"}, func(name string) error {
		removed = append(removed, name)
		return nil
	}))
	assert.Equal(t, []string{"old"}, removed)
}

func TestWritesCleanupObserveRecordsWhatItWouldDelete(t *testing.T) {
	owned := NewOwnership("writes-cleanup", map[string][]string{"profiles": {"3", "4"}})
	w := &Writes{App: "writes-cleanup", Observe: true}
	labels := map[string]string{"3": "anime", "4": "kids", "5": "hand-made"}

	require.NoError(t, w.Cleanup(owned, "profiles", []string{"3", "5"}, func(id string) string { return labels[id] }, func(string) error {
		require.Fail(t, "observe mode must not delete")
		return nil
	}))
	assert.Equal(t, []string{"profiles(anime)"}, w.Drift())
	assert.Equal(t, map[string][]string{"profiles": {"3", "4"}}, owned.Managed(), "observe mode changed ownership")
}

func TestWritesCleanupObserveHoldsToThePruneLimit(t *testing.T) {
	ids := make([]string, 0, DefaultMaxPruneCount+1)
	for i := range DefaultMaxPruneCount + 1 {
		ids = append(ids, strconv.Itoa(i))
	}
	owned := NewOwnership("writes-cleanup", map[string][]string{"profiles": ids})
	w := &Writes{App: "writes-cleanup", Observe: true}

	err := w.Cleanup(owned, "profiles", ids, func(id string) string { return id }, func(string) error { return nil })
	require.ErrorContains(t, err, "refusing to delete 26 profiles")
	assert.Empty(t, w.Drift())
}

func TestWritesSyncWritesOnlyWhatDiffers(t *testing.T) {
	w := &Writes{App: "writes-sync"}
	var wrote []string
	write := func(name string) func(map[string]any) error {
		return func(map[string]any) error { wrote = append(wrote, name); return nil }
	}

	require.NoError(t, w.Sync("settings", "same", map[string]any{"a": 1.0, "b": "x"}, map[string]any{"a": 1}, write("same")))
	require.NoError(t, w.Sync("settings", "changed", map[string]any{"a": 1.0}, map[string]any{"a": 2}, write("changed")))
	require.NoError(t, w.Sync("settings", "missing", nil, map[string]any{}, write("missing")))

	assert.Equal(t, []string{"changed", "missing"}, wrote)
}

func TestWritesSyncObserveRecordsTheDifference(t *testing.T) {
	w := &Writes{App: "writes-sync-observe", Observe: true}
	require.NoError(t, w.Sync("settings", "main", map[string]any{"a": 1.0}, map[string]any{"a": 2}, func(map[string]any) error {
		require.Fail(t, "observe mode must not write")
		return nil
	}))
	assert.Equal(t, []string{"settings(main)"}, w.Drift())
}
