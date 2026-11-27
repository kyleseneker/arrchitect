package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stagesApp is an app that keeps what's posted to it, by path.
type stagesApp struct {
	mu     sync.Mutex
	items  map[string][]map[string]any
	lists  map[string]int
	nextID int
}

func (a *stagesApp) client(t *testing.T) *HTTPClient {
	a.items, a.lists = map[string][]map[string]any{}, map[string]int{}
	return newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			a.lists[r.URL.Path]++
			items := a.items[r.URL.Path]
			if items == nil {
				items = []map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(items)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		a.nextID++
		body["id"] = float64(a.nextID)
		a.items[r.URL.Path] = append(a.items[r.URL.Path], body)
		_ = json.NewEncoder(w).Encode(body)
	})
}

func stagesDefinition() AppDefinition {
	return AppDefinition{Resources: []ResourceEndpoint{
		{Name: "tags", Path: "/tags", MatchField: "label", Policy: CreateOrUpdate},
		{Name: "indexers", Path: "/indexers", MatchField: "name", Policy: CreateOrUpdate},
		{Name: "clients", Path: "/clients", MatchField: "name", Policy: CreateOrUpdate},
	}}
}

// taggedItem builds one item named name, with the IDs of the tags it names.
func taggedItem(name string, labels ...string) Builder {
	return Builder{Count: 1, Tagged: true, Build: func(tags NamedIDs) ([]map[string]any, error) {
		ids, err := tags.ResolveTags(labels)
		if err != nil {
			return nil, err
		}
		return []map[string]any{{"name": name, "tags": ids}}, nil
	}}
}

func TestStagesNameWhatEarlierStagesCreated(t *testing.T) {
	app := &stagesApp{}
	hc := app.client(t)
	stages := Stages{
		Order: [][]string{{"tags"}, {"indexers", "clients"}},
		Builders: map[string]Builder{
			"tags": {Count: 1, Build: func(NamedIDs) ([]map[string]any, error) {
				return []map[string]any{{"label": "tv"}}, nil
			}},
			"indexers": taggedItem("indexer", "TV"),
			"clients":  taggedItem("client", "tv"),
		},
		TagPath: "/tags",
	}

	problems := stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", SyncPolicy{}, nil))
	require.Empty(t, problems)
	tag := app.items["/tags"][0]["id"]
	assert.Equal(t, []any{tag}, app.items["/indexers"][0]["tags"])
	assert.Equal(t, []any{tag}, app.items["/clients"][0]["tags"])
}

func TestStagesReadTagsOnce(t *testing.T) {
	app := &stagesApp{}
	hc := app.client(t)
	app.items["/tags"] = []map[string]any{{"id": 4, "label": "tv"}}
	stages := Stages{
		Order:    [][]string{{"indexers"}, {"clients"}},
		Builders: map[string]Builder{"indexers": taggedItem("indexer", "tv"), "clients": taggedItem("client", "tv")},
		TagPath:  "/tags",
	}

	require.Empty(t, stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", SyncPolicy{}, nil)))
	assert.Equal(t, 1, app.lists["/tags"])
}

func TestStagesLeaveOutAKindThatCantBeBuilt(t *testing.T) {
	app := &stagesApp{}
	hc := app.client(t)
	stages := Stages{
		Order: [][]string{{"indexers", "clients"}},
		Builders: map[string]Builder{
			"indexers": taggedItem("indexer", "missing"),
			"clients": {Count: 1, Build: func(NamedIDs) ([]map[string]any, error) {
				return []map[string]any{{"name": "client"}}, nil
			}},
		},
		TagPath: "/tags",
	}

	problems := stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", SyncPolicy{}, nil))
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], `the app has no item named "missing"`)
	assert.Empty(t, app.items["/indexers"])
	assert.Len(t, app.items["/clients"], 1)
}

func TestStagesReadTagsOnlyForTaggedItems(t *testing.T) {
	app := &stagesApp{}
	hc := app.client(t)
	stages := Stages{
		Order: [][]string{{"indexers", "clients"}},
		Builders: map[string]Builder{
			"indexers": {Tagged: true},
			"clients": {Count: 1, Build: func(NamedIDs) ([]map[string]any, error) {
				return []map[string]any{{"name": "client"}}, nil
			}},
		},
		TagPath: "/tags",
	}

	require.Empty(t, stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", SyncPolicy{}, nil)))
	assert.Zero(t, app.lists["/tags"])
}

func TestStagesReportATagListFailureAgainstEachTaggedKind(t *testing.T) {
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tags" {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})
	stages := Stages{
		Order:    [][]string{{"indexers", "clients"}},
		Builders: map[string]Builder{"indexers": taggedItem("indexer", "tv"), "clients": taggedItem("client", "tv")},
		TagPath:  "/tags",
	}

	problems := stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", SyncPolicy{}, nil))
	require.Len(t, problems, 2)
	assert.True(t, strings.HasPrefix(problems[0], "indexers: "), problems[0])
	assert.True(t, strings.HasPrefix(problems[1], "clients: "), problems[1])
}

func TestStagesPlanDeclaredTagsInObserveMode(t *testing.T) {
	app := &stagesApp{}
	hc := app.client(t)
	var got []int
	stages := Stages{
		Order: [][]string{{"indexers"}},
		Builders: map[string]Builder{"indexers": {Count: 1, Tagged: true, Build: func(tags NamedIDs) ([]map[string]any, error) {
			var err error
			got, err = tags.ResolveTags([]string{"tv"})
			return nil, err
		}}},
		TagPath:     "/tags",
		PlannedTags: []string{"TV"},
	}

	require.Empty(t, stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", SyncPolicy{Observe: true}, nil)))
	assert.Equal(t, []int{PlannedID}, got)
}
