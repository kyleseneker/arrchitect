package subtitles

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	bazarrclient "github.com/kyleseneker/arrchitect/internal/client/bazarr"
	"github.com/kyleseneker/arrchitect/internal/controller/reconciler"
)

func storedProfile(id int, name string) map[string]any {
	return map[string]any{
		"profileId": float64(id), "name": name, "cutoff": nil, "items": []any{},
		"mustContain": []any{}, "mustNotContain": []any{}, "originalFormat": false, "tag": nil,
	}
}

func sentProfileIDs(t *testing.T, app *fakeBazarr) []float64 {
	t.Helper()
	raw, ok := app.formValue(profilesField)
	require.True(t, ok, "no language profiles were saved")
	var sent []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &sent))
	ids := make([]float64, 0, len(sent))
	for _, p := range sent {
		ids = append(ids, p["profileId"].(float64))
	}
	return ids
}

func profilesConfig(url string, ids ...int) *subtitlesv1alpha1.Bazarr {
	cfg := bazarrConfig(url)
	cfg.Spec.Languages = &subtitlesv1alpha1.BazarrLanguages{}
	for _, id := range ids {
		cfg.Spec.Languages.Profiles = append(cfg.Spec.Languages.Profiles, subtitlesv1alpha1.BazarrLanguageProfile{
			ProfileID: new(int32(id)), Name: "declared", Items: []subtitlesv1alpha1.BazarrLanguageItem{{Language: "en"}},
		})
	}
	return cfg
}

func bazarrManaged(t *testing.T, cfg *subtitlesv1alpha1.Bazarr) map[string][]string {
	t.Helper()
	c := runBazarr(t, cfg, bazarrSecrets())
	var got subtitlesv1alpha1.Bazarr
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "bazarr", Namespace: "media"}, &got))
	return got.Status.GetManagedResources()
}

func TestBazarrKeepsProfilesItDoesNotDeclare(t *testing.T) {
	app := &fakeBazarr{profiles: []map[string]any{storedProfile(2, "hand-made")}}

	managed := bazarrManaged(t, profilesConfig(app.serve(t), 1))

	assert.ElementsMatch(t, []float64{1, 2}, sentProfileIDs(t, app), "saving the declared profile deleted a hand-made one")
	assert.Equal(t, []string{"1"}, managed[profilesKind])
}

func TestBazarrKeepsWhatADeclaredProfileLeavesOut(t *testing.T) {
	stored := storedProfile(1, "old name")
	stored["cutoff"] = float64(1)
	stored["mustContain"] = []any{"WEB"}
	stored["originalFormat"] = float64(1)
	app := &fakeBazarr{profiles: []map[string]any{stored}}

	runBazarr(t, profilesConfig(app.serve(t), 1), bazarrSecrets())

	raw, ok := app.formValue(profilesField)
	require.True(t, ok, "the renamed profile was not saved")
	var sent []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &sent))
	require.Len(t, sent, 1)
	assert.Equal(t, "declared", sent[0]["name"])
	assert.Equal(t, float64(1), sent[0]["cutoff"])
	assert.Equal(t, []any{"WEB"}, sent[0]["mustContain"])
	assert.Equal(t, float64(1), sent[0]["originalFormat"])
}

func TestBazarrDeclaredProfileSettingsReplaceBazarrs(t *testing.T) {
	stored := storedProfile(1, "declared")
	stored["cutoff"] = float64(1)
	stored["mustContain"] = []any{"WEB"}
	stored["tag"] = "old"
	app := &fakeBazarr{profiles: []map[string]any{stored}}
	cfg := profilesConfig(app.serve(t), 1, 2)
	cfg.Spec.Languages.Profiles[0].Cutoff = "any"
	cfg.Spec.Languages.Profiles[0].MustContain = []string{"BluRay"}
	cfg.Spec.Languages.Profiles[0].Tag = "english"

	runBazarr(t, cfg, bazarrSecrets())

	raw, ok := app.formValue(profilesField)
	require.True(t, ok, "the profiles were not saved")
	var sent []map[string]any
	require.NoError(t, json.Unmarshal([]byte(raw), &sent))
	require.Len(t, sent, 2)
	assert.Equal(t, float64(bazarrclient.AnyCutoff), sent[0]["cutoff"])
	assert.Equal(t, []any{"BluRay"}, sent[0]["mustContain"])
	assert.Equal(t, "english", sent[0]["tag"])
	assert.Equal(t, []any{}, sent[0]["mustNotContain"], "an unset setting must keep Bazarr's")

	assert.Nil(t, sent[1]["cutoff"])
	assert.Contains(t, sent[1], "cutoff", "a new profile needs every key Bazarr reads")
	assert.Equal(t, []any{}, sent[1]["mustContain"])
	assert.Equal(t, []any{}, sent[1]["mustNotContain"])
	assert.Equal(t, false, sent[1]["originalFormat"])
}

func TestBazarrPrunesOnlyProfilesItCreated(t *testing.T) {
	app := &fakeBazarr{profiles: []map[string]any{
		storedProfile(1, "declared"), storedProfile(2, "old"), storedProfile(3, "hand-made"),
	}}
	cfg := profilesConfig(app.serve(t), 1)
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true)}
	cfg.Status.SetManagedResources(map[string][]string{profilesKind: {"1", "2"}})

	managed := bazarrManaged(t, cfg)

	assert.ElementsMatch(t, []float64{1, 3}, sentProfileIDs(t, app))
	assert.Equal(t, []string{"1"}, managed[profilesKind])
}

func TestBazarrObserveReportsAProfilePruneWouldDelete(t *testing.T) {
	for _, tt := range []struct {
		name   string
		owned  []string
		reason string
	}{
		{"owned and undeclared", []string{"2"}, "DriftDetected"},
		{"not owned", nil, "Observed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			app := &fakeBazarr{profiles: []map[string]any{storedProfile(1, "declared"), storedProfile(2, "old")}}
			app.profiles[0]["items"] = []any{map[string]any{
				"id": float64(1), "language": "en", "hi": "False", "forced": "False", "audio_exclude": "False",
				"audio_only_include": "False",
			}}
			cfg := profilesConfig(app.serve(t), 1)
			observe := "observe"
			cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{Prune: new(true), DriftPolicy: observe}
			cfg.Status.SetManagedResources(map[string][]string{profilesKind: tt.owned})

			c := runBazarr(t, cfg, bazarrSecrets())

			_, wrote := app.formValue(profilesField)
			assert.False(t, wrote)
			var got subtitlesv1alpha1.Bazarr
			require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "bazarr", Namespace: "media"}, &got))
			synced := metaCondition(got.Status.Conditions, "Synced")
			assert.Equal(t, tt.reason, synced.Reason, synced.Message)
		})
	}
}

func TestBazarrDeletionPolicyRemovesTheProfilesItCreated(t *testing.T) {
	app := &fakeBazarr{profiles: []map[string]any{storedProfile(1, "created"), storedProfile(2, "hand-made")}}
	cfg := profilesConfig(app.serve(t), 1)
	policy := reconciler.DeletionPolicyDelete
	cfg.Spec.Reconcile = &commonv1alpha1.ReconcileConfig{DeletionPolicy: policy}
	cfg.Status.SetManagedResources(map[string][]string{profilesKind: {"1"}})
	now := metav1.NewTime(time.Now())
	cfg.DeletionTimestamp = &now
	cfg.Finalizers = []string{reconciler.Finalizer}

	runBazarr(t, cfg, bazarrSecrets())

	assert.Equal(t, []float64{2}, sentProfileIDs(t, app))
}

func metaCondition(conditions []metav1.Condition, condType string) metav1.Condition {
	for _, c := range conditions {
		if c.Type == condType {
			return c
		}
	}
	return metav1.Condition{}
}
