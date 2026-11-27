package pvr

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// delayAPI keeps delay profiles as Sonarr, Radarr and Readarr do, or as Lidarr
// does with items: a tag in one profile only, tags required on all but the
// default, and reorder renumbering the tagged profiles.
type delayAPI struct {
	mu       sync.Mutex
	items    bool
	tags     []string
	profiles []map[string]any
	writes   []string
	nextID   float64
	// undeletable is a profile the app refuses to delete.
	undeletable float64
}

func newDelayAPI(items bool, tags ...string) *delayAPI {
	a := &delayAPI{items: items, tags: tags, nextID: 1}
	fallback := map[string]any{
		"id": float64(1), "order": maxOrder, "tags": []any{},
		"enableUsenet": true, "enableTorrent": true, "preferredProtocol": "usenet",
		"usenetDelay": float64(0), "torrentDelay": float64(0),
		"bypassIfHighestQuality": true, "bypassIfAboveCustomFormatScore": false, "minimumCustomFormatScore": float64(0),
	}
	if items {
		for _, key := range []string{"enableUsenet", "enableTorrent", "preferredProtocol", "usenetDelay", "torrentDelay"} {
			delete(fallback, key)
		}
		fallback["name"] = "Default"
		fallback["items"] = delayItems()
	}
	a.profiles = []map[string]any{fallback}
	return a
}

func delayItems() []any {
	return []any{
		map[string]any{"name": "Usenet", "protocol": "UsenetDownloadProtocol", "allowed": true, "delay": float64(0)},
		map[string]any{"name": "Torrent", "protocol": "TorrentDownloadProtocol", "allowed": true, "delay": float64(0)},
	}
}

func (a *delayAPI) add(tags []any, fields map[string]any) float64 {
	a.nextID++
	profile := map[string]any{"id": a.nextID, "order": float64(len(a.profiles)), "tags": tags, "bypassIfHighestQuality": false}
	if a.items {
		profile["items"] = delayItems()
	} else {
		profile["enableUsenet"], profile["enableTorrent"], profile["preferredProtocol"] = true, true, "usenet"
		profile["usenetDelay"], profile["torrentDelay"] = float64(0), float64(0)
	}
	maps.Copy(profile, fields)
	a.profiles = append(a.profiles, profile)
	return a.nextID
}

// find returns the profile with id. The caller holds the lock.
func (a *delayAPI) find(id float64) map[string]any {
	for _, p := range a.profiles {
		if p["id"] == id {
			return p
		}
	}
	return nil
}

func (a *delayAPI) refuse(w http.ResponseWriter, body map[string]any, id float64) bool {
	tags, _ := body["tags"].([]any)
	if id != 1 && len(tags) == 0 {
		http.Error(w, `[{"errorMessage":"'Tags' must not be empty."}]`, http.StatusBadRequest)
		return true
	}
	for _, other := range a.profiles {
		if other["id"] == id {
			continue
		}
		for _, tag := range other["tags"].([]any) {
			if slices.Contains(tags, tag) {
				http.Error(w, `[{"errorMessage":"One or more tags is used in another profile"}]`, http.StatusBadRequest)
				return true
			}
		}
	}
	return false
}

func (a *delayAPI) reorder(id, after float64) {
	tagged := slices.DeleteFunc(slices.Clone(a.profiles), func(p map[string]any) bool { return p["id"] == float64(1) })
	slices.SortFunc(tagged, func(x, y map[string]any) int { return int(x["order"].(float64) - y["order"].(float64)) })
	moved := a.find(id)
	tagged = slices.DeleteFunc(tagged, func(p map[string]any) bool { return p["id"] == id })
	at := slices.IndexFunc(tagged, func(p map[string]any) bool { return p["id"] == after }) + 1
	tagged = slices.Insert(tagged, at, moved)
	for i, p := range tagged {
		p["order"] = float64(i + 1)
	}
}

func (a *delayAPI) client(t *testing.T) *engine.HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		resource := strings.TrimPrefix(r.URL.Path, "/api/v3/")
		switch {
		case r.Method == http.MethodGet && resource == "tag":
			tags := make([]map[string]any, 0, len(a.tags))
			for i, label := range a.tags {
				tags = append(tags, map[string]any{"id": float64(i + 1), "label": label})
			}
			_ = json.NewEncoder(w).Encode(tags)
			return
		case r.Method == http.MethodGet && resource == "delayprofile/schema" && a.items:
			_ = json.NewEncoder(w).Encode(map[string]any{"items": delayItems(), "order": 0, "tags": []any{}})
			return
		case r.Method == http.MethodGet && resource == "delayprofile":
			_ = json.NewEncoder(w).Encode(a.profiles)
			return
		case r.Method == http.MethodGet:
			http.NotFound(w, r)
			return
		}
		a.writes = append(a.writes, r.Method+" "+resource)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		parts := strings.Split(resource, "/")
		id, _ := strconv.ParseFloat(parts[len(parts)-1], 64)
		switch {
		case r.Method == http.MethodPut && len(parts) == 3 && parts[1] == "reorder":
			param := "after"
			if a.items {
				param = "afterId"
			}
			after, _ := strconv.ParseFloat(r.URL.Query().Get(param), 64)
			a.reorder(id, after)
			_ = json.NewEncoder(w).Encode(a.profiles)
		case r.Method == http.MethodPost:
			if a.refuse(w, body, 0) {
				return
			}
			fields := map[string]any{}
			for k, v := range body {
				if k != "tags" && k != "order" && k != "id" {
					fields[k] = v
				}
			}
			created := a.add(body["tags"].([]any), fields)
			_ = json.NewEncoder(w).Encode(a.find(created))
		case r.Method == http.MethodPut:
			if a.refuse(w, body, id) {
				return
			}
			current := a.find(id)
			order := current["order"]
			clear(current)
			maps.Copy(current, body)
			current["order"] = order
			_ = json.NewEncoder(w).Encode(current)
		case r.Method == http.MethodDelete && id == a.undeletable:
			http.Error(w, `{"message":"Profile is in use"}`, http.StatusBadRequest)
		case r.Method == http.MethodDelete:
			a.profiles = slices.DeleteFunc(a.profiles, func(p map[string]any) bool { return p["id"] == id })
		}
	}))
	t.Cleanup(srv.Close)
	hc, err := engine.NewHTTPClient(srv.URL, engine.AuthNone, engine.WithTransport(&http.Transport{}))
	require.NoError(t, err)
	return hc
}

func (a *delayAPI) reconcile(t *testing.T, hc *engine.HTTPClient, declared []commonv1alpha1.DelayProfile, policy engine.SyncPolicy, managed map[string][]string) outcome {
	t.Helper()
	pass := engine.NewPass(hc.AppLabel(), "", policy, managed)
	errs := reconcileDelayProfiles(context.Background(), hc, "v3", declared, pass, nil)
	return outcome{
		Synced: pass.SyncedItems(), Errors: messages(errs), Drift: pass.Drift(), Pruned: pass.Owned.Pruned(), Managed: pass.Owned.Managed(),
	}
}

// profile returns a copy of the profile with id.
func (a *delayAPI) profile(id float64) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return maps.Clone(a.find(id))
}

// byTags returns a copy of the profile holding exactly tags.
func (a *delayAPI) byTags(tags ...float64) map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, p := range a.profiles {
		got := profileTags(p)
		if len(got) == len(tags) && overlap(got, intIDs(tags)) == len(tags) {
			return maps.Clone(p)
		}
	}
	return nil
}

// count returns how many profiles the app holds.
func (a *delayAPI) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.profiles)
}

// writeLog returns the writes so far.
func (a *delayAPI) writeLog() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return slices.Clone(a.writes)
}

func intIDs(ids []float64) []int {
	out := make([]int, 0, len(ids))
	for _, id := range ids {
		out = append(out, int(id))
	}
	return out
}

func TestDelayProfilesCreateUpdateTheDefaultAndConverge(t *testing.T) {
	app := newDelayAPI(false, "anime", "kids")
	hc := app.client(t)
	declared := []commonv1alpha1.DelayProfile{
		{TagLabels: []string{"kids", "anime"}, PreferredProtocol: "torrent", UsenetDelay: new(int32(60))},
		{TorrentDelay: new(int32(30)), BypassIfHighestQuality: new(false)},
	}

	result := app.reconcile(t, hc, declared, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	created := app.byTags(1, 2)
	require.NotNil(t, created)
	assert.Equal(t, "torrent", created["preferredProtocol"])
	assert.InDelta(t, 60, created["usenetDelay"], 0)
	assert.Equal(t, true, created["enableUsenet"], "a new profile starts from the app's defaults")
	fallback := app.profile(1)
	assert.InDelta(t, 30, fallback["torrentDelay"], 0)
	assert.Equal(t, false, fallback["bypassIfHighestQuality"])
	assert.Equal(t, "usenet", fallback["preferredProtocol"], "undeclared settings stay as the app has them")
	assert.Equal(t, map[string][]string{delayProfilesKind: {delayProfileID(created)}}, result.Managed)
	assert.ElementsMatch(t, []string{"delayProfiles(kids,anime)", "delayProfiles(default)"}, result.Synced)

	writes := len(app.writeLog())
	second := app.reconcile(t, hc, declared, engine.SyncPolicy{}, result.Managed)
	require.Empty(t, second.Errors)
	assert.Len(t, app.writeLog(), writes, "second pass must not write")
	assert.Equal(t, result.Managed, second.Managed)
}

func TestDelayProfilesAdoptTheProfileHoldingADeclaredTag(t *testing.T) {
	app := newDelayAPI(false, "anime", "kids")
	handMade := app.add([]any{float64(2), float64(1)}, nil)
	hc := app.client(t)

	result := app.reconcile(t, hc, []commonv1alpha1.DelayProfile{{TagLabels: []string{"anime", "kids"}, TorrentDelay: new(int32(5))}}, engine.SyncPolicy{Prune: true}, nil)
	require.Empty(t, result.Errors)
	assert.Equal(t, []string{"PUT delayprofile/" + strconv.Itoa(int(handMade))}, app.writeLog())
	assert.Equal(t, []any{float64(2), float64(1)}, app.profile(handMade)["tags"], "the same tags in another order are left alone")
	assert.Empty(t, result.Managed, "an adopted profile is not the operator's to delete")
}

func TestDelayProfilesChangeTagsInPlace(t *testing.T) {
	app := newDelayAPI(false, "anime", "kids", "cartoons")
	hc := app.client(t)
	first := app.reconcile(t, hc, []commonv1alpha1.DelayProfile{{TagLabels: []string{"anime"}}}, engine.SyncPolicy{}, nil)
	require.Empty(t, first.Errors)
	id := app.byTags(1)["id"]

	second := app.reconcile(t, hc, []commonv1alpha1.DelayProfile{{TagLabels: []string{"anime", "cartoons"}}}, engine.SyncPolicy{Prune: true}, first.Managed)
	require.Empty(t, second.Errors)
	assert.Equal(t, id, app.byTags(1, 3)["id"], "adding a tag updates the profile instead of replacing it")
	assert.Equal(t, first.Managed, second.Managed)
}

func TestDelayProfilesMoveATagBetweenProfilesInOnePass(t *testing.T) {
	app := newDelayAPI(false, "anime", "kids", "cartoons")
	a := app.add([]any{float64(1)}, nil)
	b := app.add([]any{float64(2), float64(3)}, nil)
	hc := app.client(t)

	result := app.reconcile(t, hc, []commonv1alpha1.DelayProfile{
		{TagLabels: []string{"anime", "cartoons"}},
		{TagLabels: []string{"kids"}},
	}, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	assert.Equal(t, a, app.byTags(1, 3)["id"])
	assert.Equal(t, b, app.byTags(2)["id"])
}

func TestDelayProfilesPruneFreesTagsForANewProfile(t *testing.T) {
	app := newDelayAPI(false, "anime", "kids", "four")
	owned := app.add([]any{float64(1)}, nil)
	handMade := app.add([]any{float64(3)}, nil)
	hc := app.client(t)
	managed := map[string][]string{delayProfilesKind: {strconv.Itoa(int(owned))}}

	result := app.reconcile(t, hc, []commonv1alpha1.DelayProfile{{TagLabels: []string{"kids"}}}, engine.SyncPolicy{Prune: true}, managed)
	require.Empty(t, result.Errors)
	assert.Nil(t, app.profile(owned), "the owned profile left the spec")
	assert.NotNil(t, app.profile(handMade), "a profile the operator didn't create stays")
	assert.Equal(t, []engine.PrunedResource{{Type: delayProfilesKind, Name: strconv.Itoa(int(owned))}}, result.Pruned)

	app2 := newDelayAPI(false, "anime")
	owned2 := app2.add([]any{float64(1)}, nil)
	hc2 := app2.client(t)
	moved := app2.reconcile(t, hc2, []commonv1alpha1.DelayProfile{{TagLabels: []string{"anime"}, UsenetDelay: new(int32(10))}}, engine.SyncPolicy{Prune: true},
		map[string][]string{delayProfilesKind: {strconv.Itoa(int(owned2))}})
	require.Empty(t, moved.Errors)
	assert.Equal(t, owned2, app2.byTags(1)["id"], "a profile still declared by its tags is kept")
}

func TestDelayProfilesObserveWritesNothing(t *testing.T) {
	app := newDelayAPI(false, "anime", "kids", "four")
	stale := app.add([]any{float64(3)}, nil)
	second := app.add([]any{float64(2)}, nil)
	first := app.add([]any{float64(1)}, nil)
	hc := app.client(t)
	managed := map[string][]string{delayProfilesKind: {strconv.Itoa(int(stale)), strconv.Itoa(int(first)), strconv.Itoa(int(second))}}

	result := app.reconcile(t, hc, []commonv1alpha1.DelayProfile{
		{TagLabels: []string{"anime"}},
		{TagLabels: []string{"kids"}, TorrentDelay: new(int32(20))},
		{UsenetDelay: new(int32(5))},
	}, engine.SyncPolicy{Prune: true, Observe: true}, managed)
	require.Empty(t, result.Errors)
	assert.Empty(t, app.writeLog())
	assert.ElementsMatch(t, []string{
		"delayProfiles(four)", "delayProfiles(kids)", "delayProfiles(default)", "delayProfiles(order)",
	}, result.Drift)
	assert.Equal(t, managed, result.Managed)
}

func TestDelayProfilesPruneKeepsARefusedProfileAndDeletesTheRest(t *testing.T) {
	app := newDelayAPI(false, "anime", "kids", "four")
	refused := app.add([]any{float64(1)}, nil)
	deletable := app.add([]any{float64(2)}, nil)
	app.undeletable = refused
	hc := app.client(t)
	managed := map[string][]string{delayProfilesKind: {strconv.Itoa(int(refused)), strconv.Itoa(int(deletable))}}

	result := app.reconcile(t, hc, []commonv1alpha1.DelayProfile{{TagLabels: []string{"four"}}}, engine.SyncPolicy{Prune: true}, managed)
	require.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "refused to delete it: Profile is in use")
	assert.NotNil(t, app.profile(refused))
	assert.Nil(t, app.profile(deletable), "a refusal stopped the deletions after it")
	assert.Equal(t, map[string][]string{delayProfilesKind: {strconv.Itoa(int(refused))}}, result.Managed, "the refused profile stays owned")
}

func TestDelayProfilesObserveRefusesTheSamePruneAsEnforce(t *testing.T) {
	for _, observe := range []bool{false, true} {
		t.Run(strconv.FormatBool(observe), func(t *testing.T) {
			app := newDelayAPI(false)
			owned := make([]string, 0, engine.DefaultMaxPruneCount+1)
			for i := range engine.DefaultMaxPruneCount + 1 {
				owned = append(owned, strconv.Itoa(int(app.add([]any{float64(i + 1)}, nil))))
			}
			hc := app.client(t)

			result := app.reconcile(t, hc, []commonv1alpha1.DelayProfile{{UsenetDelay: new(int32(5))}},
				engine.SyncPolicy{Prune: true, Observe: observe}, map[string][]string{delayProfilesKind: owned})
			require.Len(t, result.Errors, 1)
			assert.Contains(t, result.Errors[0], "refusing to delete 26")
			assert.Empty(t, app.writeLog())
			assert.Empty(t, result.Drift)
		})
	}
}

func TestDelayProfilesFollowTheDeclaredOrder(t *testing.T) {
	app := newDelayAPI(false, "a", "b", "c", "own")
	c := app.add([]any{float64(3)}, nil)
	handMade := app.add([]any{float64(4)}, nil)
	a := app.add([]any{float64(1)}, nil)
	b := app.add([]any{float64(2)}, nil)
	hc := app.client(t)
	declared := []commonv1alpha1.DelayProfile{{TagLabels: []string{"a"}}, {TagLabels: []string{"b"}}, {TagLabels: []string{"c"}}}

	result := app.reconcile(t, hc, declared, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	order := func(id float64) float64 { return app.profile(id)["order"].(float64) }
	assert.Less(t, order(a), order(b))
	assert.Less(t, order(b), order(c))
	assert.Less(t, order(handMade), order(c), "the moved profile goes after the one before it, not to the top")
	assert.Equal(t, maxOrder, order(1))

	writes := len(app.writeLog())
	require.Empty(t, app.reconcile(t, hc, declared, engine.SyncPolicy{}, nil).Errors)
	assert.Len(t, app.writeLog(), writes)
}

func TestDelayProfilesWithProtocolItems(t *testing.T) {
	app := newDelayAPI(true, "favorites", "live")
	live := app.add([]any{float64(2)}, nil)
	hc := app.client(t)
	declared := []commonv1alpha1.DelayProfile{
		{TagLabels: []string{"favorites"}, PreferredProtocol: "torrent", EnableUsenet: new(false), TorrentDelay: new(int32(15))},
		{TagLabels: []string{"live"}},
		{UsenetDelay: new(int32(30))},
	}

	result := app.reconcile(t, hc, declared, engine.SyncPolicy{}, nil)
	require.Empty(t, result.Errors)
	created := app.byTags(1)
	assert.Less(t, created["order"], app.profile(live)["order"])
	assert.Equal(t, []any{
		map[string]any{"name": "Torrent", "protocol": "TorrentDownloadProtocol", "allowed": true, "delay": float64(15)},
		map[string]any{"name": "Usenet", "protocol": "UsenetDownloadProtocol", "allowed": false, "delay": float64(0)},
	}, created["items"])
	assert.NotContains(t, created, "preferredProtocol")
	assert.Equal(t, []any{
		map[string]any{"name": "Usenet", "protocol": "UsenetDownloadProtocol", "allowed": true, "delay": float64(30)},
		map[string]any{"name": "Torrent", "protocol": "TorrentDownloadProtocol", "allowed": true, "delay": float64(0)},
	}, app.profile(1)["items"])

	writes := len(app.writeLog())
	require.Empty(t, app.reconcile(t, hc, declared, engine.SyncPolicy{}, result.Managed).Errors)
	assert.Len(t, app.writeLog(), writes, "second pass must not write")
}

func TestDelayProfilesRefuseWhatTheAppWouldReject(t *testing.T) {
	tests := []struct {
		name     string
		declared []commonv1alpha1.DelayProfile
		want     string
	}{
		{"unknown tag", []commonv1alpha1.DelayProfile{{TagLabels: []string{"missing"}}}, `delayProfiles(missing): tag: the app has no item named "missing"`},
		{"shared tag", []commonv1alpha1.DelayProfile{{TagLabels: []string{"anime"}}, {TagLabels: []string{"kids", "anime"}}},
			"delayProfiles(kids,anime): shares a tag with anime"},
		{"two defaults", []commonv1alpha1.DelayProfile{{}, {}}, "delayProfiles(default): only one profile can be without tags"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newDelayAPI(false, "anime", "kids")
			hc := app.client(t)
			result := app.reconcile(t, hc, tt.declared, engine.SyncPolicy{Prune: true}, nil)
			require.Len(t, result.Errors, 1)
			assert.Contains(t, result.Errors[0], tt.want)
			assert.Empty(t, app.writeLog())
		})
	}
}

func TestDelayProfileCleanupDeletesOnlyOwnedProfiles(t *testing.T) {
	app := newDelayAPI(false, "anime", "kids")
	owned := app.add([]any{float64(1)}, nil)
	app.add([]any{float64(2)}, nil)
	hc := app.client(t)

	ownership := engine.NewOwnership("sonarr", map[string][]string{delayProfilesKind: {strconv.Itoa(int(owned))}})
	require.NoError(t, engine.DeleteManagedResources(context.Background(), hc, Definition(Sonarr), ownership))
	deleted := ownership.Pruned()
	assert.Equal(t, []string{"DELETE delayprofile/" + strconv.Itoa(int(owned))}, app.writeLog())
	require.Len(t, deleted, 1)
	assert.Equal(t, 2, app.count())
}
