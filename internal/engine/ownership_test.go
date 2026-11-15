package engine

import (
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kyleseneker/arrchitect/internal/metrics"
)

func TestOwnershipPrunesOnlyOwnedUndeclaredItems(t *testing.T) {
	pruned := metrics.ResourcesPrunedTotal.WithLabelValues("ownership-test", "feeds")
	before := testutil.ToFloat64(pruned)
	o := NewOwnership("ownership-test", map[string][]string{"feeds": {"old", "kept"}})

	var removed []string
	require.NoError(t, o.Prune("feeds", []string{"kept"}, []string{"old", "kept", "hand-made"}, func(name string) error {
		removed = append(removed, name)
		return nil
	}))

	assert.Equal(t, []string{"old"}, removed, "only an owned item that left the resource is deleted")
	assert.Equal(t, map[string][]string{"feeds": {"kept"}}, o.Managed())
	assert.Equal(t, []PrunedResource{{Type: "feeds", Name: "old"}}, o.Pruned())
	assert.Equal(t, before+1, testutil.ToFloat64(pruned))
}

func TestOwnershipLeavesAKindWithNothingDeclared(t *testing.T) {
	o := NewOwnership("ownership-test", map[string][]string{"feeds": {"old"}})
	assert.Empty(t, o.Candidates("feeds", nil, []string{"old"}))
	require.NoError(t, o.Prune("feeds", nil, []string{"old"}, func(string) error {
		require.Fail(t, "a kind with nothing declared was pruned")
		return nil
	}))
}

func TestOwnershipCleanupDeletesEveryOwnedItem(t *testing.T) {
	o := NewOwnership("ownership-test", map[string][]string{"feeds": {"a", "b", "gone"}})
	var removed []string
	require.NoError(t, o.Cleanup("feeds", []string{"b", "a", "hand-made"}, func(name string) error {
		removed = append(removed, name)
		return nil
	}))
	assert.Equal(t, []string{"a", "b"}, removed)
	assert.Equal(t, map[string][]string{"feeds": {"gone"}}, o.Managed(), "an owned item already gone stays recorded")
}

func TestOwnershipKeepsWhatItFailedToDelete(t *testing.T) {
	o := NewOwnership("ownership-test", map[string][]string{"feeds": {"a", "b"}})
	err := o.Prune("feeds", []string{"c"}, []string{"a", "b", "c"}, func(name string) error {
		if name == "b" {
			return errors.New("app said no")
		}
		return nil
	})
	require.EqualError(t, err, `deleting feeds "b": app said no`)
	assert.Equal(t, map[string][]string{"feeds": {"b"}}, o.Managed(), "a failed delete keeps ownership for the next pass")
}

func TestOwnershipCleanupKeepsWhatTheAppRefusesAndCarriesOn(t *testing.T) {
	o := NewOwnership("ownership-test", map[string][]string{"profiles": {"a", "b", "c"}})
	var removed []string
	err := o.Cleanup("profiles", []string{"a", "b", "c"}, func(name string) error {
		if name == "b" {
			return &APIError{StatusCode: 409, Body: []byte(`{"message":"in use by a root folder"}`)}
		}
		removed = append(removed, name)
		return nil
	})

	var kept *KeptError
	require.ErrorAs(t, err, &kept)
	assert.Equal(t, "ownership-test", kept.App)
	assert.Equal(t, []KeptResource{{Type: "profiles", Name: "b", Reason: "in use by a root folder"}}, kept.Kept)
	assert.Equal(t, []string{"a", "c"}, removed)

	err = o.Prune("profiles", []string{"x"}, []string{"b"}, func(string) error {
		return &APIError{StatusCode: 409, Body: []byte(`{"message":"in use"}`)}
	})
	assert.ErrorContains(t, err, `deleting profiles "b"`, "prune reports a refusal each pass instead of keeping it")
}

func TestSplitKeptSeparatesRefusalsFromRetryableFailures(t *testing.T) {
	sonarr := &KeptError{App: "seerr", Kept: []KeptResource{{Name: "a"}}}
	radarr := &KeptError{App: "seerr", Kept: []KeptResource{{Name: "b"}}}

	kept, rest := SplitKept(errors.Join(sonarr, nil, fmt.Errorf("cleaning up: %w", radarr)))
	assert.Equal(t, []*KeptError{sonarr, radarr}, kept)
	require.NoError(t, rest)

	kept, rest = SplitKept(errors.Join(sonarr, errors.New("timeout")))
	assert.Equal(t, []*KeptError{sonarr}, kept)
	assert.EqualError(t, rest, "timeout")

	kept, rest = SplitKept(nil)
	assert.Empty(t, kept)
	assert.NoError(t, rest)
}

func TestOwnershipRefusesPastTheThreshold(t *testing.T) {
	existing := make([]string, 0, DefaultMaxPruneCount+1)
	for i := range DefaultMaxPruneCount + 1 {
		existing = append(existing, fmt.Sprintf("f%d", i))
	}
	o := NewOwnership("ownership-test", map[string][]string{"feeds": existing})
	err := o.Prune("feeds", []string{"declared"}, existing, func(string) error {
		require.Fail(t, "deleted past the threshold")
		return nil
	})
	require.ErrorContains(t, err, "refusing to delete 26 feeds")
}

func TestOwnershipRecordsWhatItCreatesOnce(t *testing.T) {
	o := NewOwnership("ownership-test", nil)
	o.Created("feeds", "a")
	o.Created("feeds", "a")
	assert.Equal(t, map[string][]string{"feeds": {"a"}}, o.Managed())
}
