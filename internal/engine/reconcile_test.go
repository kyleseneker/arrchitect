package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

func TestIsNilInterface(t *testing.T) {
	tests := []struct {
		name string
		val  any
		want bool
	}{
		{"nil", nil, true},
		{"typed nil pointer", (*string)(nil), true},
		{"non-nil pointer", new("hello"), false},
		{"non-pointer value", "hello", false},
		{"integer", 42, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNilInterface(tt.val))
		})
	}
}

func TestFailedUpdateDoesNotPruneOrForgetOwnership(t *testing.T) {
	var deletes int
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "desired", "enabled": false}, {"id": 2, "name": "removed"}})
		case http.MethodPut:
			w.WriteHeader(http.StatusInternalServerError)
		case http.MethodDelete:
			deletes++
		}
	})
	previous := map[string][]string{"indexers": {"desired", "removed"}, "tags": {"retained"}}
	result := run(context.Background(), hc, ownershipDefinition(), nil,
		map[string][]map[string]any{"indexers": {{"name": "desired", "enabled": true}}}, SyncPolicy{Prune: true}, previous)
	require.False(t, result.Success())
	assert.Zero(t, deletes, "a failed apply must block pruning for the resource type")
	assert.Equal(t, previous, result.Managed, "failed writes and omitted sections must retain ownership")
}

func TestExistingResourcesAreNotAdoptedForPruning(t *testing.T) {
	for _, observe := range []bool{false, true} {
		for _, existing := range []bool{false, true} {
			t.Run(strconv.FormatBool(observe)+"/"+strconv.FormatBool(existing), func(t *testing.T) {
				var posts int
				hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost {
						posts++
						return
					}
					items := []map[string]any{}
					if existing {
						items = append(items, map[string]any{"id": 1, "name": "manual"})
					}
					_ = json.NewEncoder(w).Encode(items)
				})
				result := run(context.Background(), hc, ownershipDefinition(), nil,
					map[string][]map[string]any{"indexers": {{"name": "manual"}}}, SyncPolicy{Observe: observe}, nil)
				require.True(t, result.Success())
				if !observe && !existing {
					assert.Equal(t, 1, posts)
					assert.Equal(t, []string{"manual"}, result.Managed["indexers"])
				} else {
					assert.Zero(t, posts)
					assert.Empty(t, result.Managed["indexers"])
				}
			})
		}
	}
}

func TestPruneRetainsFailedDeletesForRetry(t *testing.T) {
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			if r.URL.Path == "/indexers/3" {
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "desired"}, {"id": 2, "name": "deleted"}, {"id": 3, "name": "retry"}})
	})
	previous := map[string][]string{"indexers": {"desired", "deleted", "retry"}}
	result := run(context.Background(), hc, ownershipDefinition(), nil,
		map[string][]map[string]any{"indexers": {{"name": "desired"}}}, SyncPolicy{Prune: true}, previous)
	require.False(t, result.Success())
	assert.Equal(t, []string{"desired", "retry"}, result.Managed["indexers"])
	assert.Equal(t, []string{"desired", "deleted", "retry"}, previous["indexers"], "input status must not be mutated")
	require.Len(t, result.Pruned, 1)
	assert.Equal(t, "deleted", result.Pruned[0].Name)
}

func TestObserveReportsWhatPruneWouldDelete(t *testing.T) {
	var writes []string
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes = append(writes, r.Method+" "+r.URL.Path)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "desired"}, {"id": 2, "name": "old"}, {"id": 3, "name": "hand-made"}})
	})
	previous := map[string][]string{"indexers": {"desired", "old"}}
	result := run(context.Background(), hc, ownershipDefinition(), nil,
		map[string][]map[string]any{"indexers": {{"name": "desired"}}}, SyncPolicy{Prune: true, Observe: true}, previous)

	require.True(t, result.Success(), result.Message())
	assert.Empty(t, writes)
	assert.Equal(t, []string{"indexers(old)"}, result.Drift)
	assert.Equal(t, previous, result.Managed)
}

func ownershipDefinition() AppDefinition {
	return AppDefinition{Resources: []ResourceEndpoint{{Name: "indexers", Path: "/indexers", MatchField: "name", Policy: CreateOrUpdate, Prunable: true}}}
}

func TestMaskedSecretsAreIsolatedBetweenInstances(t *testing.T) {
	for _, sameSecret := range []bool{true, false} {
		t.Run(fmt.Sprint(sameSecret), func(t *testing.T) {
			var writesA, writesB []string
			a := maskingServer(t, "shared-name", &writesA)
			b := maskingServer(t, "shared-name", &writesB)
			ep := ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate}
			passwordB := "password-a"
			if !sameSecret {
				passwordB = "password-b"
			}
			for range 3 {
				require.NoError(t, reconcileResource(context.Background(), a, ep, desiredWithPassword("shared-name", "password-a"), false))
				require.NoError(t, reconcileResource(context.Background(), b, ep, desiredWithPassword("shared-name", passwordB), false))
			}
			assert.Len(t, writesA, 1)
			assert.Len(t, writesB, 1, "each instance must receive exactly one initial write")
		})
	}
}

func TestMaskedSecretCacheIncludesOwner(t *testing.T) {
	var writes []string
	hc := maskingServer(t, "recreated", &writes)
	ep := ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate}
	prefix := "Sonarr/ns/" + t.Name() + "/"
	t.Cleanup(func() { reconciler.ForgetSecrets(prefix) })
	for _, owner := range []string{prefix + "uid-1", prefix + "uid-1", prefix + "uid-2"} {
		pass := NewPass(hc.AppLabel(), owner, SyncPolicy{}, nil)
		require.NoError(t, reconcileWith(context.Background(), hc, pass, ep, desiredWithPassword("recreated", "password")))
	}
	assert.Len(t, writes, 2, "a recreated CR must seed its secret independently")
}

func TestDeleteManagedResourcesRespectsOwnershipAndDependencyOrder(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(strconv.FormatBool(fail), func(t *testing.T) {
			var deleted []string
			hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodDelete {
					deleted = append(deleted, r.URL.Path)
					if fail {
						w.WriteHeader(500)
					}
					return
				}
				_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "owned"}, {"id": 2, "name": "manual"}})
			})
			def := AppDefinition{Resources: []ResourceEndpoint{
				{Name: "tags", Path: "/tags", MatchField: "name"},
				{Name: "formats", Path: "/formats", MatchField: "name", Prunable: true},
				{Name: "profiles", Path: "/profiles", MatchField: "name", Prunable: true},
			}}
			managed := map[string][]string{"tags": {"owned"}, "formats": {"owned"}, "profiles": {"owned"}}
			owned := NewOwnership("test", managed)
			err := DeleteManagedResources(context.Background(), hc, def, owned)
			pruned := owned.Pruned()
			if fail {
				require.Error(t, err)
				assert.Equal(t, []string{"/profiles/1"}, deleted)
				assert.Empty(t, pruned)
			} else {
				require.NoError(t, err)
				assert.Equal(t, []string{"/profiles/1", "/formats/1"}, deleted)
				assert.Len(t, pruned, 2)
			}
		})
	}
}

func TestDeleteManagedResourcesKeepsWhatTheAppRefusesAndCarriesOn(t *testing.T) {
	var deleted []string
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			if r.URL.Path == "/profiles/1" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"Profile [Music] is in use."}`))
				return
			}
			deleted = append(deleted, r.URL.Path)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 1, "name": "owned"}})
	})
	def := AppDefinition{Resources: []ResourceEndpoint{
		{Name: "formats", Path: "/formats", MatchField: "name", Prunable: true},
		{Name: "profiles", Path: "/profiles", MatchField: "name", Prunable: true},
	}}
	owned := NewOwnership("test", map[string][]string{"formats": {"owned"}, "profiles": {"owned"}})
	refused, err := SplitKept(DeleteManagedResources(context.Background(), hc, def, owned))

	require.NoError(t, err)
	assert.Equal(t, []string{"/formats/1"}, deleted)
	require.Len(t, owned.Pruned(), 1)
	require.Len(t, refused, 1)
	require.Len(t, refused[0].Kept, 1)
	assert.Equal(t, "profiles", refused[0].Kept[0].Type)
	assert.Equal(t, "test refused to delete it: Profile [Music] is in use.", refused[0].Kept[0].Reason)
}

func TestRefusalLeavesRetryableFailuresOut(t *testing.T) {
	for status, refused := range map[int]bool{
		http.StatusBadRequest: true, http.StatusConflict: true, http.StatusUnauthorized: false,
		http.StatusNotFound: false, http.StatusTooManyRequests: false, http.StatusInternalServerError: false,
	} {
		_, got := Refusal(&APIError{StatusCode: status})
		assert.Equal(t, refused, got, "status %d", status)
	}
	_, got := Refusal(errors.New("connection refused"))
	assert.False(t, got)
}

// Observe mode reports what differs in the result, so status can say so; a
// masked secret it never saw written is not one of them.
func TestReconcileAppReportsObservedDrift(t *testing.T) {
	var writes []string
	hc := maskingServer(t, "qbit", &writes)
	def := AppDefinition{Resources: []ResourceEndpoint{{
		Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate,
	}}}
	observe := SyncPolicy{Observe: true}

	matching := run(context.Background(), hc, def, nil,
		map[string][]map[string]any{"downloadClients": {desiredWithPassword("qbit", "never-written")}}, observe, nil)
	assert.Empty(t, matching.Drift, "a masked password observe never wrote is not drift")

	missing := run(context.Background(), hc, def, nil,
		map[string][]map[string]any{"downloadClients": {desiredWithPassword("sab", "pw")}}, observe, nil)
	assert.Equal(t, []string{"downloadClients(sab)"}, missing.Drift)
	assert.Empty(t, missing.Synced, "a difference is not reported as synced")
	assert.Empty(t, writes, "observe wrote to the app")
}

func TestReconcileSetting_NoChange(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 1, "rename": true, "createEmpty": false,
			})
		case http.MethodPut:
			assert.Fail(t, "PUT should not be called when nothing changed")
		}
	}

	hc := newTestServer(t, handler)
	err := reconcileSetting(context.Background(), hc, NewPass("test", "", SyncPolicy{}, nil), namingSetting, map[string]any{
		"rename": true, "createEmpty": false,
	})
	assert.NoError(t, err)
}

func TestReconcileSetting_WithChange(t *testing.T) {
	var putCalled bool
	var putBody map[string]any

	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": 1, "rename": false, "extra": "preserved",
			})
		case http.MethodPut:
			putCalled = true
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			assert.True(t, strings.HasSuffix(r.URL.Path, "/1"), "PUT path should include id")
			w.WriteHeader(http.StatusOK)
		}
	}

	hc := newTestServer(t, handler)
	err := reconcileSetting(context.Background(), hc, NewPass("test", "", SyncPolicy{}, nil), namingSetting, map[string]any{
		"rename": true,
	})
	require.NoError(t, err)
	assert.True(t, putCalled)
	assert.Equal(t, true, putBody["rename"])
	assert.Equal(t, "preserved", putBody["extra"])
}

func TestReconcileSetting_NoID(t *testing.T) {
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"rename": true})
	})
	err := reconcileSetting(context.Background(), hc, NewPass("test", "", SyncPolicy{}, nil), namingSetting, map[string]any{"rename": true})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no id")
}

func TestReconcileResource_CreateNew(t *testing.T) {
	var postCalled bool

	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case http.MethodPost:
			postCalled = true
			w.WriteHeader(http.StatusCreated)
		}
	}

	hc := newTestServer(t, handler)
	endpoint := ResourceEndpoint{Name: "tags", Path: "/api/v3/tag", MatchField: "label", Policy: CreateOrUpdate}
	err := reconcileResource(context.Background(), hc, endpoint, map[string]any{"label": "test"}, false)
	require.NoError(t, err)
	assert.True(t, postCalled)
}

func TestReconcileResource_UpdateExisting(t *testing.T) {
	var putCalled bool

	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": float64(5), "name": "test-dc", "host": "old-host"},
			})
		case http.MethodPut:
			putCalled = true
			assert.True(t, strings.HasSuffix(r.URL.Path, "/5"))
			w.WriteHeader(http.StatusOK)
		}
	}

	hc := newTestServer(t, handler)
	endpoint := ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate}
	err := reconcileResource(context.Background(), hc, endpoint, map[string]any{
		"name": "test-dc", "host": "new-host",
	}, false)
	require.NoError(t, err)
	assert.True(t, putCalled)
}

func TestCreateDefaultsGoToNewItemsOnly(t *testing.T) {
	existing := []map[string]any{}
	var posted, put map[string]any
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(existing)
		case http.MethodPost:
			require.NoError(t, json.NewDecoder(r.Body).Decode(&posted))
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			require.NoError(t, json.NewDecoder(r.Body).Decode(&put))
		}
	})
	endpoint := ResourceEndpoint{
		Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate,
		CreateDefaults: map[string]any{"enable": true, "priority": 1, "tags": []any{}},
	}
	desired := map[string]any{"name": "qBit", "priority": 5}

	require.NoError(t, reconcileResource(context.Background(), hc, endpoint, desired, false))
	assert.Equal(t, map[string]any{"name": "qBit", "enable": true, "priority": float64(5), "tags": []any{}}, posted)

	existing = []map[string]any{{"id": 3, "name": "qBit", "enable": false, "priority": 2, "tags": []any{7}}}
	require.NoError(t, reconcileResource(context.Background(), hc, endpoint, desired, false))
	assert.Equal(t, map[string]any{"id": float64(3), "name": "qBit", "enable": false, "priority": float64(5), "tags": []any{float64(7)}}, put,
		"an existing item keeps what the resource leaves out")
	assert.Equal(t, map[string]any{"enable": true, "priority": 1, "tags": []any{}}, endpoint.CreateDefaults)
}

func TestExactListsRemoveEntriesTheResourceLeavesOut(t *testing.T) {
	current := map[string]any{"id": float64(4), "name": "x265", "specifications": []any{
		map[string]any{"name": "HEVC", "negate": false, "fields": []any{map[string]any{"name": "value", "value": "x265", "label": "Regex"}}},
		map[string]any{"name": "Not Remux", "negate": true},
	}}
	var puts []map[string]any
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{current})
		case http.MethodPut:
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			puts = append(puts, body)
			current = body
		}
	})
	endpoint := ResourceEndpoint{
		Name: "customFormats", Path: "/api/v3/customformat", MatchField: "name", Policy: CreateOrUpdate,
		ExactLists: []string{"specifications"},
	}
	desired := map[string]any{"name": "x265", "specifications": []map[string]any{
		{"name": "HEVC", "fields": []map[string]any{{"name": "value", "value": "x265"}}},
	}}

	require.NoError(t, reconcileResource(context.Background(), hc, endpoint, desired, false))
	require.Len(t, puts, 1)
	assert.Equal(t, []any{map[string]any{
		"name": "HEVC", "negate": false, "fields": []any{map[string]any{"name": "value", "value": "x265", "label": "Regex"}},
	}}, puts[0]["specifications"])

	require.NoError(t, reconcileResource(context.Background(), hc, endpoint, desired, false))
	assert.Len(t, puts, 1, "a trimmed list settles")

	delete(desired, "specifications")
	current["specifications"] = append(current["specifications"].([]any), map[string]any{"name": "Made In UI"})
	require.NoError(t, reconcileResource(context.Background(), hc, endpoint, desired, false))
	assert.Len(t, puts, 1, "an undeclared list is left alone")
}

func TestObserveReportsAnEntryAnExactListWouldRemove(t *testing.T) {
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		_ = json.NewEncoder(w).Encode([]map[string]any{{"id": 4, "name": "x265", "specifications": []any{
			map[string]any{"name": "HEVC"}, map[string]any{"name": "Not Remux"},
		}}})
	})
	endpoint := ResourceEndpoint{
		Name: "customFormats", Path: "/api/v3/customformat", MatchField: "name", Policy: CreateOrUpdate,
		ExactLists: []string{"specifications"},
	}
	pass := NewPass("test", "", SyncPolicy{Observe: true}, nil)
	require.NoError(t, reconcileWith(context.Background(), hc, pass, endpoint,
		map[string]any{"name": "x265", "specifications": []map[string]any{{"name": "HEVC"}}}))
	assert.Equal(t, []string{"customFormats(x265)"}, pass.Drift())
}

func TestReconcileResource_CreateOnly(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": float64(1), "path": "/tv"},
			})
		case http.MethodPut:
			assert.Fail(t, "PUT should not be called for CreateOnly policy")
		case http.MethodPost:
			assert.Fail(t, "POST should not be called when resource exists with CreateOnly")
		}
	}

	hc := newTestServer(t, handler)
	endpoint := ResourceEndpoint{Name: "rootFolders", Path: "/api/v3/rootfolder", MatchField: "path", Policy: CreateOnly}
	err := reconcileResource(context.Background(), hc, endpoint, map[string]any{"path": "/tv"}, false)
	assert.NoError(t, err)
}

func TestReconcileResource_UpdateOnly(t *testing.T) {
	var puts []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": float64(3), "name": "DVD", "maxSize": float64(100)}})
		case http.MethodPut:
			puts = append(puts, r.URL.Path)
		case http.MethodPost:
			assert.Fail(t, "POST should never be called for UpdateOnly policy")
		}
	}

	hc := newTestServer(t, handler)
	endpoint := ResourceEndpoint{Name: "qualityDefinitions", Path: "/api/v3/qualitydefinition", MatchField: "name", Policy: UpdateOnly}
	require.NoError(t, reconcileResource(context.Background(), hc, endpoint, map[string]any{"name": "DVD", "maxSize": 50}, false))
	assert.Equal(t, []string{"/api/v3/qualitydefinition/3"}, puts)

	for _, observe := range []bool{false, true} {
		err := reconcileResource(context.Background(), hc, endpoint, map[string]any{"name": "Laserdisc"}, observe)
		require.ErrorContains(t, err, "qualityDefinitions can't be created")
	}
}

func TestPruneResources(t *testing.T) {
	var deletedPaths []string

	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": float64(1), "name": "managed"},
				{"id": float64(2), "name": "unmanaged-1"},
				{"id": float64(3), "name": "unmanaged-2"},
			})
		case http.MethodDelete:
			deletedPaths = append(deletedPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}
	}

	hc := newTestServer(t, handler)
	endpoint := ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate, Prunable: true}
	desired := []map[string]any{{"name": "managed"}}

	before := testutil.ToFloat64(metrics.ResourcesPrunedTotal.WithLabelValues(testAppLabel, "downloadClients"))
	pruned, err := pruneResources(context.Background(), hc, endpoint, desired, []string{"managed", "unmanaged-1", "unmanaged-2"})
	require.NoError(t, err)
	assert.Len(t, pruned, 2)
	assert.Contains(t, deletedPaths, "/api/v3/downloadclient/2")
	assert.Contains(t, deletedPaths, "/api/v3/downloadclient/3")

	after := testutil.ToFloat64(metrics.ResourcesPrunedTotal.WithLabelValues(testAppLabel, "downloadClients"))
	assert.Equal(t, before+2, after, "expected prune counter to increment by 2")
}

func TestPruneResources_SafetyThreshold(t *testing.T) {
	// Create more than DefaultMaxPruneCount (25) unmanaged resources
	existing := make([]map[string]any, 30)
	for i := range existing {
		existing[i] = map[string]any{"id": float64(i + 1), "name": fmt.Sprintf("item-%d", i+1)}
	}

	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(existing)
		case http.MethodDelete:
			assert.Fail(t, "DELETE should not be called when safety threshold exceeded")
		}
	}

	hc := newTestServer(t, handler)
	endpoint := ResourceEndpoint{Name: "indexers", Path: "/api/v3/indexer", MatchField: "name", Prunable: true}
	desired := []map[string]any{{"name": "kept"}} // all 30 would be pruned

	before := testutil.ToFloat64(metrics.ResourcesPrunedTotal.WithLabelValues(testAppLabel, "indexers"))
	allManaged := make([]string, len(existing))
	for i := range existing {
		allManaged[i] = existing[i]["name"].(string)
	}
	_, err := pruneResources(context.Background(), hc, endpoint, desired, allManaged)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to delete")
	assert.Contains(t, err.Error(), "30")

	after := testutil.ToFloat64(metrics.ResourcesPrunedTotal.WithLabelValues(testAppLabel, "indexers"))
	assert.Equal(t, before, after, "prune counter must NOT increment when safety threshold blocks deletion")
}

func TestReconcileApp(t *testing.T) {
	callLog := make(map[string]int)

	handler := func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		callLog[key]++

		switch {
		// Settings: naming
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/config/naming":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": float64(1), "rename": false})
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v3/config/naming/"):
			w.WriteHeader(http.StatusOK)

		// Resources: tags
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/tag":
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/tag":
			w.WriteHeader(http.StatusCreated)

		default:
			w.WriteHeader(http.StatusOK)
		}
	}

	hc := newTestServer(t, handler)

	def := AppDefinition{
		Settings: []SettingEndpoint{
			{Name: "naming", Path: "/api/v3/config/naming"},
		},
		Resources: []ResourceEndpoint{
			{Name: "tags", Path: "/api/v3/tag", MatchField: "label", Policy: CreateOrUpdate},
		},
	}
	sections := map[string]any{
		"naming": map[string]any{"rename": true},
	}
	resources := map[string][]map[string]any{
		"tags": {{"label": "test"}},
	}

	result := run(context.Background(), hc, def, sections, resources, SyncPolicy{}, nil)
	assert.True(t, result.Success())
	assert.Contains(t, result.Synced, "naming")
	assert.Contains(t, result.Synced, "tags(test)")
}

func TestReconcileApp_SkipsNilSections(t *testing.T) {
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Failf(t, "no HTTP calls expected", "got %s %s", r.Method, r.URL.Path)
	})

	def := AppDefinition{
		Settings: []SettingEndpoint{{Name: "naming", Path: "/api/v3/config/naming"}},
	}
	result := run(context.Background(), hc, def, map[string]any{}, nil, SyncPolicy{}, nil)
	assert.True(t, result.Success())
	assert.Empty(t, result.Synced)
}

func TestPruneSkipsResourcesTheOperatorNeverCreated(t *testing.T) {
	var deletedPaths []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": float64(1), "name": "operator-made"},
				{"id": float64(2), "name": "hand-built-by-human"},
			})
		case http.MethodDelete:
			deletedPaths = append(deletedPaths, r.URL.Path)
			w.WriteHeader(http.StatusOK)
		}
	}
	hc := newTestServer(t, handler)
	endpoint := ResourceEndpoint{Name: "indexers", Path: "/api/v3/indexer", MatchField: "name", Prunable: true}

	// The CR no longer declares either one, but the operator only ever created
	// "operator-made". The hand-built indexer must survive.
	pruned, err := pruneResources(context.Background(), hc, endpoint,
		[]map[string]any{{"name": "declared"}}, []string{"operator-made"})
	require.NoError(t, err)

	assert.Len(t, pruned, 1)
	assert.Equal(t, "operator-made", pruned[0].Name)
	assert.Contains(t, deletedPaths, "/api/v3/indexer/1")
	assert.NotContains(t, deletedPaths, "/api/v3/indexer/2",
		"a resource the operator never created must never be pruned")
}

func TestPruneIsNoOpOnFirstReconcile(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": float64(1), "name": "pre-existing"},
			})
		case http.MethodDelete:
			assert.Fail(t, "nothing may be deleted when the operator has no recorded managed set")
		}
	}
	hc := newTestServer(t, handler)
	endpoint := ResourceEndpoint{Name: "indexers", Path: "/api/v3/indexer", MatchField: "name", Prunable: true}

	pruned, err := pruneResources(context.Background(), hc, endpoint, []map[string]any{{"name": "declared"}}, nil)
	require.NoError(t, err)
	assert.Empty(t, pruned)
}

func driftServer(t *testing.T, existing map[string]any, writes *[]string) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]map[string]any{existing})
			return
		}
		if writes != nil {
			*writes = append(*writes, r.Method+" "+r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(existing)
	}))
	t.Cleanup(srv.Close)

	hc, err := NewHTTPClient(srv.URL, AuthAPIKey, WithAPIKey("k"),
		WithAPIKeyHeader("X-Api-Key"), WithAppLabel("sonarr"),
		WithTransport(&http.Transport{}))
	require.NoError(t, err, "building client")
	return hc
}

func endpointFor() ResourceEndpoint {
	return ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate}
}

// A resource whose live state already matches must not be rewritten, so the
// counter stays flat when nothing has drifted.
func TestNoDriftLeavesCounterUnchanged(t *testing.T) {
	metrics.DriftCorrectedTotal.Reset()
	existing := map[string]any{"id": float64(1), "name": "qbit", "enable": true}
	hc := driftServer(t, existing, nil)

	require.NoError(t, reconcileResource(context.Background(), hc, endpointFor(), map[string]any{"name": "qbit", "enable": true}, false), "reconcile")
	assert.Zero(t, testutil.CollectAndCount(metrics.DriftCorrectedTotal), "counter moved despite no drift")
}

// A resource the operator has to rewrite increments the counter, so a correction
// that never sticks shows up as a steady rate rather than passing unnoticed.
func TestDriftCorrectionIncrementsCounter(t *testing.T) {
	metrics.DriftCorrectedTotal.Reset()
	existing := map[string]any{"id": float64(1), "name": "qbit", "enable": false}
	hc := driftServer(t, existing, nil)

	require.NoError(t, reconcileResource(context.Background(), hc, endpointFor(), map[string]any{"name": "qbit", "enable": true}, false), "reconcile")
	assert.Equal(t, 1.0, testutil.ToFloat64(metrics.DriftCorrectedTotal.WithLabelValues("sonarr", "downloadClients", "qbit")), "drift_corrected_total")
}

// Observe must record the difference and leave the app untouched, so drift can be
// measured before the operator is given write authority.
func TestObserveRecordsDriftWithoutWriting(t *testing.T) {
	metrics.DriftObservedTotal.Reset()
	var writes []string
	existing := map[string]any{"id": float64(1), "name": "qbit", "enable": false}
	hc := driftServer(t, existing, &writes)

	require.NoError(t, reconcileResource(context.Background(), hc, endpointFor(), map[string]any{"name": "qbit", "enable": true}, true), "reconcile")

	assert.Equal(t, 1.0, testutil.ToFloat64(metrics.DriftObservedTotal.WithLabelValues("sonarr", "downloadClients", "qbit")), "drift_observed_total")
	assert.Empty(t, writes, "observe wrote to the app")
}

// Enforce is the default, so an unset policy still corrects drift.
func TestEnforceWritesTheCorrection(t *testing.T) {
	metrics.DriftCorrectedTotal.Reset()
	var writes []string
	existing := map[string]any{"id": float64(1), "name": "qbit", "enable": false}
	hc := driftServer(t, existing, &writes)

	require.NoError(t, reconcileResource(context.Background(), hc, endpointFor(), map[string]any{"name": "qbit", "enable": true}, false), "reconcile")
	assert.Len(t, writes, 1, "expected one write")
}

// maskingServer mimics the apps: it stores whatever is written but always
// reports the secret field back as "********".
func maskingServer(t *testing.T, name string, writes *[]string) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"id": float64(1), "name": name, "enable": true,
			"fields": []any{
				map[string]any{"name": "host", "value": "qbit.svc"},
				map[string]any{"name": "password", "value": "********"},
			},
		}
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]map[string]any{body})
			return
		}
		*writes = append(*writes, r.Method+" "+r.URL.Path)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	hc, err := NewHTTPClient(srv.URL, AuthAPIKey, WithAPIKey("k"),
		WithAPIKeyHeader("X-Api-Key"), WithAppLabel("sonarr"),
		WithTransport(&http.Transport{}))
	require.NoError(t, err, "building client")
	return hc
}

func desiredWithPassword(name, pw string) map[string]any {
	return map[string]any{
		"name": name, "enable": true,
		"fields": []any{
			map[string]any{"name": "host", "value": "qbit.svc"},
			map[string]any{"name": "password", "value": pw},
		},
	}
}

// The app masks the password on read, so the operator once rewrote it on every
// pass and the write never settled. It must seed the secret once and then go
// quiet.
func TestMaskedSecretSettlesAfterOneWrite(t *testing.T) {
	metrics.DriftCorrectedTotal.Reset()
	var writes []string
	hc := maskingServer(t, "settles", &writes)
	ep := ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate}

	for i := range 5 {
		require.NoError(t, reconcileResource(context.Background(), hc, ep, desiredWithPassword("settles", "pw-v1"), false), "reconcile %d", i)
	}

	assert.Len(t, writes, 1, "the masked secret is still looping")
}

// A rotated secret is invisible in the fetched state, so it has to be pushed
// on the strength of the remembered digest alone.
func TestRotatedSecretIsWritten(t *testing.T) {
	metrics.DriftCorrectedTotal.Reset()
	var writes []string
	hc := maskingServer(t, "rotates", &writes)
	ep := ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate}

	ctx := context.Background()
	for range 3 {
		require.NoError(t, reconcileResource(ctx, hc, ep, desiredWithPassword("rotates", "pw-v1"), false), "reconcile")
	}
	require.Len(t, writes, 1, "before rotation")

	for range 3 {
		require.NoError(t, reconcileResource(ctx, hc, ep, desiredWithPassword("rotates", "pw-v2"), false), "reconcile after rotation")
	}
	assert.Len(t, writes, 2, "after rotation")
}

// Observe must not seed the secret, so a CR being measured stays read-only.
func TestObserveDoesNotRecordMaskedSecret(t *testing.T) {
	metrics.DriftCorrectedTotal.Reset()
	var writes []string
	hc := maskingServer(t, "observed", &writes)
	ep := ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate}

	for range 3 {
		require.NoError(t, reconcileResource(context.Background(), hc, ep, desiredWithPassword("observed", "pw-v1"), true), "reconcile")
	}
	assert.Empty(t, writes, "observe wrote to the app")
}

// Prowlarr was the worst offender at ten writes a minute. Its applications
// carry the key inside fields[], and the live resource holds fields the CR
// never declares, so the merge has to line up before the mask is even reached.
func TestProwlarrApplicationSettles(t *testing.T) {
	metrics.DriftCorrectedTotal.Reset()
	var writes []string
	live := map[string]any{
		"id": float64(1), "name": "Sonarr", "syncLevel": "fullSync",
		"implementation": "Sonarr", "configContract": "SonarrSettings",
		"fields": []any{
			map[string]any{"name": "prowlarrUrl", "value": "http://prowlarr:9696"},
			map[string]any{"name": "baseUrl", "value": "http://sonarr:8989"},
			map[string]any{"name": "apiKey", "value": "********"},
			map[string]any{"name": "authUsername", "value": nil},
			map[string]any{"name": "authPassword", "value": nil},
			map[string]any{"name": "syncCategories", "value": []any{5000, 5010}},
			map[string]any{"name": "animeSyncCategories", "value": []any{5070}},
			map[string]any{"name": "syncRejectBlocklistedTorrentHashesWhileGrabbing", "value": false},
		},
		"tags": []any{},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]map[string]any{live})
			return
		}
		writes = append(writes, r.Method+" "+r.URL.Path)
		_ = json.NewEncoder(w).Encode(live)
	}))
	t.Cleanup(srv.Close)

	hc, err := NewHTTPClient(srv.URL, AuthAPIKey, WithAPIKey("k"),
		WithAPIKeyHeader("X-Api-Key"), WithAppLabel("prowlarr"),
		WithTransport(&http.Transport{}))
	require.NoError(t, err, "building client")

	ep := ResourceEndpoint{Name: "applications", Path: "/api/v1/applications", MatchField: "name", Policy: CreateOrUpdate}
	desired := map[string]any{
		"name": "Sonarr", "syncLevel": "fullSync",
		"implementation": "Sonarr", "configContract": "SonarrSettings",
		"fields": []any{
			map[string]any{"name": "prowlarrUrl", "value": "http://prowlarr:9696"},
			map[string]any{"name": "baseUrl", "value": "http://sonarr:8989"},
			map[string]any{"name": "apiKey", "value": "real-key"},
			map[string]any{"name": "syncCategories", "value": []any{5000, 5010}},
		},
		"tags": []any{},
	}

	for i := range 5 {
		require.NoError(t, reconcileResource(context.Background(), hc, ep, desired, false), "reconcile %d", i)
	}
	assert.Len(t, writes, 1)
}

// A resource created with a secret comes back masked. Without remembering what
// the create wrote, the next pass rewrote every new download client and indexer.
func TestCreatedSecretIsNotRewrittenOnTheNextPass(t *testing.T) {
	var writes []string
	created := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"id": float64(1), "name": "created", "enable": true,
			"fields": []any{
				map[string]any{"name": "host", "value": "qbit.svc"},
				map[string]any{"name": "password", "value": "********"},
			},
		}
		if r.Method == http.MethodGet {
			if !created {
				_ = json.NewEncoder(w).Encode([]map[string]any{})
				return
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{body})
			return
		}
		writes = append(writes, r.Method)
		created = true
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	hc, err := NewHTTPClient(srv.URL, AuthAPIKey, WithAPIKey("k"), WithAppLabel("sonarr"), WithTransport(&http.Transport{}))
	require.NoError(t, err)
	ep := ResourceEndpoint{Name: "downloadClients", Path: "/api/v3/downloadclient", MatchField: "name", Policy: CreateOrUpdate}

	for i := range 3 {
		require.NoError(t, reconcileResource(context.Background(), hc, ep, desiredWithPassword("created", "pw"), false), "reconcile %d", i)
	}
	assert.Equal(t, []string{http.MethodPost}, writes, "want only the create")
}

var namingSetting = SettingEndpoint{Name: "naming", Path: "/api/v3/config/naming"}

// run is ReconcileApp with a pass built from policy and managed, its outcome
// gathered for assertions.
func run(
	ctx context.Context, hc *HTTPClient, def AppDefinition, sections map[string]any, resources map[string][]map[string]any,
	policy SyncPolicy, managed map[string][]string,
) outcome {
	pass := NewPass(hc.AppLabel(), "", policy, managed)
	errs := ReconcileApp(ctx, hc, def, sections, resources, pass)
	return outcome{
		Synced: pass.SyncedItems(), Errors: messages(errs), Drift: pass.Drift(), Pruned: pass.Owned.Pruned(), Managed: pass.Owned.Managed(),
	}
}

// reconcileWith reconciles one declared item through pass, listing the
// endpoint first as ReconcileApp does.
func reconcileWith(ctx context.Context, hc *HTTPClient, pass *Pass, endpoint ResourceEndpoint, desired map[string]any) error {
	existing, err := hc.GetJSONList(ctx, endpoint.Path)
	if err != nil {
		return err
	}
	return reconcileItem(ctx, hc, pass, endpoint, &existing, desired)
}

// reconcileResource reconciles one declared item in a pass of its own.
func reconcileResource(ctx context.Context, hc *HTTPClient, endpoint ResourceEndpoint, desired map[string]any, observe bool) error {
	return reconcileWith(ctx, hc, NewPass(hc.AppLabel(), "", SyncPolicy{Observe: observe}, nil), endpoint, desired)
}

// pruneResources prunes endpoint against desired, with managed as what the
// operator created, and returns what it deleted.
func pruneResources(
	ctx context.Context, hc *HTTPClient, endpoint ResourceEndpoint, desired []map[string]any, managed []string,
) ([]PrunedResource, error) {
	pass := NewPass(hc.AppLabel(), "", SyncPolicy{Prune: true}, map[string][]string{endpoint.Name: managed})
	existing, err := hc.GetJSONList(ctx, endpoint.Path)
	if err != nil {
		return nil, err
	}
	err = pruneEndpoint(ctx, hc, pass, endpoint, desired, existing)
	return pass.Owned.Pruned(), err
}

// outcome is a pass's result, with what it deleted and the ownership it ended
// with. Errors holds the failures' messages.
type outcome struct {
	Synced  []string
	Errors  []string
	Drift   []string
	Pruned  []PrunedResource
	Managed map[string][]string
}

func (o outcome) Success() bool { return len(o.Errors) == 0 }

func (o outcome) Message() string { return strings.Join(o.Errors, "; ") }

// messages lists the text of each error.
func messages(errs []error) []string {
	out := make([]string, 0, len(errs))
	for _, err := range errs {
		out = append(out, err.Error())
	}
	return out
}

func TestReconcileAppListsEachEndpointOncePerPass(t *testing.T) {
	var lists, creates int
	stored := []map[string]any{{"id": float64(1), "name": "existing"}}
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			lists++
			_ = json.NewEncoder(w).Encode(stored)
		case http.MethodPost:
			creates++
			var item map[string]any
			_ = json.NewDecoder(r.Body).Decode(&item)
			item["id"] = float64(10 + creates)
			_ = json.NewEncoder(w).Encode(item)
		}
	})
	def := AppDefinition{Resources: []ResourceEndpoint{{Name: "tags", Path: "/tag", MatchField: "name", Policy: CreateOrUpdate}}}
	declared := []map[string]any{{"name": "existing"}, {"name": "new"}, {"name": "new"}}

	result := run(context.Background(), hc, def, nil, map[string][]map[string]any{"tags": declared}, SyncPolicy{}, nil)

	assert.Empty(t, result.Errors)
	assert.Equal(t, 2, lists, "one list for the pass and one check before the create, not one per item")
	assert.Equal(t, 1, creates, "an item created earlier in the pass is found by a later one")
	assert.Equal(t, []string{"new"}, result.Managed["tags"])
}

func TestAnUpdateMergesOverTheItemAsItIsNow(t *testing.T) {
	lists := 0
	var put map[string]any
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			lists++
			seeders := 1.0
			if lists > 1 {
				seeders = 3 // changed in the app after the pass listed the endpoint
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": float64(7), "name": "B", "rss": false, "minimumSeeders": seeders}})
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&put)
		}
	})
	def := AppDefinition{Resources: []ResourceEndpoint{{Name: "indexers", Path: "/indexer", MatchField: "name", Policy: CreateOrUpdate}}}

	result := run(context.Background(), hc, def, nil, map[string][]map[string]any{"indexers": {{"name": "B", "rss": true}}}, SyncPolicy{}, nil)

	require.Empty(t, result.Errors)
	assert.Equal(t, true, put["rss"])
	assert.InDelta(t, 3, put["minimumSeeders"], 0, "an undeclared field keeps the value the app has now")
}

func TestPruneSparesAnItemRenamedInTheAppDuringThePass(t *testing.T) {
	lists, deleted := 0, false
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			lists++
			old := "old"
			if lists > 1 {
				old = "renamed by hand" // renamed in the app after the pass listed the endpoint
			}
			_ = json.NewEncoder(w).Encode([]map[string]any{{"id": float64(1), "name": "kept"}, {"id": float64(5), "name": old}})
		case http.MethodDelete:
			deleted = true
		}
	})
	def := AppDefinition{Resources: []ResourceEndpoint{{Name: "indexers", Path: "/indexer", MatchField: "name", Policy: CreateOrUpdate, Prunable: true}}}

	result := run(context.Background(), hc, def, nil, map[string][]map[string]any{"indexers": {{"name": "kept"}}},
		SyncPolicy{Prune: true}, map[string][]string{"indexers": {"kept", "old"}})

	require.Empty(t, result.Errors)
	assert.False(t, deleted, "the item no longer has the name the operator owns")
}

func TestPruneRefusesANameTheAppHasTwice(t *testing.T) {
	deleted := false
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = true
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": float64(1), "name": "kept"}, {"id": float64(5), "name": "old"}, {"id": float64(6), "name": "old"},
		})
	})
	endpoint := ResourceEndpoint{Name: "indexers", Path: "/indexer", MatchField: "name", Prunable: true}

	_, err := pruneResources(context.Background(), hc, endpoint, []map[string]any{{"name": "kept"}}, []string{"old"})

	require.ErrorContains(t, err, `has 2 indexers named "old"`)
	assert.False(t, deleted)
}

func TestCleanupKeepsANameTheAppHasTwiceAndCarriesOn(t *testing.T) {
	var deleted []string
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deleted = append(deleted, r.URL.Path)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": float64(1), "name": "dup"}, {"id": float64(2), "name": "dup"}, {"id": float64(3), "name": "single"},
		})
	})
	def := AppDefinition{Resources: []ResourceEndpoint{
		{Name: "formats", Path: "/formats", MatchField: "name", Prunable: true},
		{Name: "profiles", Path: "/profiles", MatchField: "name", Prunable: true},
	}}
	owned := NewOwnership("sonarr", map[string][]string{"formats": {"single"}, "profiles": {"dup", "single"}})

	refused, err := SplitKept(DeleteManagedResources(context.Background(), hc, def, owned))

	require.NoError(t, err, "no retry fixes a duplicate, so it must not block the rest")
	assert.Equal(t, []string{"/profiles/3", "/formats/3"}, deleted)
	require.Len(t, refused, 1)
	assert.Equal(t, `test-app has 2 profiles named "dup"`, refused[0].Kept[0].Reason)
}

func TestACreateChecksTheItemIsStillMissing(t *testing.T) {
	lists, creates := 0, 0
	hc := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			lists++
			items := []map[string]any{}
			if lists > 1 {
				items = append(items, map[string]any{"id": float64(4), "name": "tv"}) // created by someone else mid-pass
			}
			_ = json.NewEncoder(w).Encode(items)
		case http.MethodPost:
			creates++
		}
	})
	def := AppDefinition{Resources: []ResourceEndpoint{{Name: "tags", Path: "/tag", MatchField: "name", Policy: CreateOrUpdate}}}

	result := run(context.Background(), hc, def, nil, map[string][]map[string]any{"tags": {{"name": "tv"}}}, SyncPolicy{}, nil)

	require.Empty(t, result.Errors)
	assert.Zero(t, creates, "a second item with the same name must not be created")
	assert.Empty(t, result.Managed["tags"], "an item someone else created isn't the operator's")
}
