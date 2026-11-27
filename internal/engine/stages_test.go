package engine

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stagesApp is an app that keeps what's posted to it, by path. A GET of a path
// in failures fails while its count lasts.
type stagesApp struct {
	mu       sync.Mutex
	items    map[string][]map[string]any
	lists    map[string]int
	failures map[string]int
	nextID   int
}

func (a *stagesApp) client(t *testing.T) *HTTPClient {
	a.items, a.lists = map[string][]map[string]any{}, map[string]int{}
	return newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			a.lists[r.URL.Path]++
			if a.failures[r.URL.Path] > 0 {
				a.failures[r.URL.Path]--
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
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

// seed sets what the app holds at path.
func (a *stagesApp) seed(path string, items ...map[string]any) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.items[path] = items
}

// snapshot copies what the app holds and how often each path was listed.
func (a *stagesApp) snapshot() (map[string][]map[string]any, map[string]int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	items := make(map[string][]map[string]any, len(a.items))
	for path, list := range a.items {
		items[path] = slices.Clone(list)
	}
	return items, maps.Clone(a.lists)
}

func stagesDefinition() AppDefinition {
	return AppDefinition{Resources: []ResourceEndpoint{
		{Name: "tags", Path: "/tags", MatchField: "label", Policy: CreateOrUpdate},
		{Name: "indexers", Path: "/indexers", MatchField: "name", Policy: CreateOrUpdate},
		{Name: "clients", Path: "/clients", MatchField: "name", Policy: CreateOrUpdate},
		{Name: "feeds", Path: "/feeds", MatchField: "name", Policy: CreateOrUpdate},
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

	problems := stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", "", SyncPolicy{}, nil))
	require.Empty(t, problems)
	items, _ := app.snapshot()
	tag := items["/tags"][0]["id"]
	assert.Equal(t, []any{tag}, items["/indexers"][0]["tags"])
	assert.Equal(t, []any{tag}, items["/clients"][0]["tags"])
}

func TestStagesReadTagsOncePerStage(t *testing.T) {
	app := &stagesApp{}
	hc := app.client(t)
	app.seed("/tags", map[string]any{"id": 4, "label": "tv"})
	stages := Stages{
		Order: [][]string{{"indexers", "clients"}, {"feeds"}},
		Builders: map[string]Builder{
			"indexers": taggedItem("indexer", "tv"), "clients": taggedItem("client", "tv"), "feeds": taggedItem("feed", "tv"),
		},
		TagPath: "/tags",
	}

	require.Empty(t, stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", "", SyncPolicy{}, nil)))
	_, lists := app.snapshot()
	assert.Equal(t, 2, lists["/tags"])
}

func TestStagesReadTagsAgainAfterAStageFailedToRead(t *testing.T) {
	app := &stagesApp{failures: map[string]int{"/tags": 1}}
	hc := app.client(t)
	app.seed("/tags", map[string]any{"id": 4, "label": "tv"})
	stages := Stages{
		Order:    [][]string{{"indexers"}, {"clients"}},
		Builders: map[string]Builder{"indexers": taggedItem("indexer", "tv"), "clients": taggedItem("client", "tv")},
		TagPath:  "/tags",
	}

	problems := stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", "", SyncPolicy{}, nil))
	require.Len(t, problems, 1)
	assert.True(t, strings.HasPrefix(problems[0].Error(), "indexers: "), problems[0].Error())
	items, _ := app.snapshot()
	assert.Empty(t, items["/indexers"])
	assert.Len(t, items["/clients"], 1)
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

	problems := stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", "", SyncPolicy{}, nil))
	require.Len(t, problems, 1)
	assert.ErrorContains(t, problems[0], `the app has no item named "missing"`)
	items, _ := app.snapshot()
	assert.Empty(t, items["/indexers"])
	assert.Len(t, items["/clients"], 1)
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

	require.Empty(t, stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", "", SyncPolicy{}, nil)))
	_, lists := app.snapshot()
	assert.Zero(t, lists["/tags"])
}

func TestStagesReportATagListFailureAgainstEachTaggedKind(t *testing.T) {
	app := &stagesApp{failures: map[string]int{"/tags": 1}}
	hc := app.client(t)
	stages := Stages{
		Order:    [][]string{{"indexers", "clients"}},
		Builders: map[string]Builder{"indexers": taggedItem("indexer", "tv"), "clients": taggedItem("client", "tv")},
		TagPath:  "/tags",
	}

	problems := stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", "", SyncPolicy{}, nil))
	require.Len(t, problems, 2)
	assert.True(t, strings.HasPrefix(problems[0].Error(), "indexers: "), problems[0].Error())
	assert.True(t, strings.HasPrefix(problems[1].Error(), "clients: "), problems[1].Error())
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

	require.Empty(t, stages.Reconcile(context.Background(), hc, stagesDefinition(), NewPass("test", "", SyncPolicy{Observe: true}, nil)))
	assert.Equal(t, []int{PlannedID}, got)
}
