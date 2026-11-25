package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeKind lists items and records what is listed and deleted.
type fakeKind struct {
	items   []map[string]any
	listed  int
	removed []int
}

func (f *fakeKind) kind(name string) Kind {
	return Kind{
		Name: name,
		List: func(context.Context) ([]map[string]any, error) { f.listed++; return f.items, nil },
		Remove: func(_ context.Context, id int) error {
			f.removed = append(f.removed, id)
			return nil
		},
	}
}

func TestPruneKindsDeletesOnlyOwnedUndeclaredItems(t *testing.T) {
	feeds := &fakeKind{items: []map[string]any{
		{"id": float64(1), "name": "old"}, {"id": float64(2), "name": "kept"}, {"id": float64(3), "name": "hand-made"},
	}}
	pass := NewPass("kinds", SyncPolicy{Prune: true}, map[string][]string{"feeds": {"old", "kept"}})

	problems := pass.PruneKinds(context.Background(), []Kind{feeds.kind("feeds")}, map[string][]string{"feeds": {"kept"}}, nil)

	assert.Empty(t, problems)
	assert.Equal(t, []int{1}, feeds.removed)
}

func TestPruneKindsNeverListsWhatItCantPrune(t *testing.T) {
	unowned, failed, undeclared := &fakeKind{}, &fakeKind{}, &fakeKind{}
	pass := NewPass("kinds", SyncPolicy{Prune: true}, map[string][]string{"failed": {"a"}, "undeclared": {"a"}})

	problems := pass.PruneKinds(context.Background(),
		[]Kind{unowned.kind("unowned"), failed.kind("failed"), undeclared.kind("undeclared")},
		map[string][]string{"unowned": {"x"}, "failed": {"x"}}, map[string]bool{"failed": true})

	assert.Empty(t, problems)
	assert.Zero(t, unowned.listed, "a kind the operator owns nothing of may be out of a scoped key's reach")
	assert.Zero(t, failed.listed, "a kind a write failed for waits for a later pass")
	assert.Zero(t, undeclared.listed, "leaving a list out of a resource never empties it")
}

func TestCleanupKindsDeletesOwnedItemsInOrder(t *testing.T) {
	lists := &fakeKind{items: []map[string]any{{"id": float64(5), "title": "mine"}, {"id": float64(6), "title": "theirs"}}}
	clients := &fakeKind{items: []map[string]any{{"id": float64(7), "name": "qbit"}}}
	owned := NewOwnership("kinds", map[string][]string{"lists": {"mine"}})
	listKind := lists.kind("lists")
	listKind.NameField = "title"

	require.NoError(t, owned.CleanupKinds(context.Background(), []Kind{listKind, clients.kind("downloadClients")}))

	assert.Equal(t, []int{5}, lists.removed)
	assert.Zero(t, clients.listed)
}
