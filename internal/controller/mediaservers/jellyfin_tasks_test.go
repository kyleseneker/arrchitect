package mediaservers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	jellyfinclient "github.com/kyleseneker/arrchitect/internal/client/jellyfin"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/testnet"
)

// fakeJellyfinTasks keeps scheduled tasks as Jellyfin does: a POST replaces a
// task's triggers, and a trigger's unset fields are left out of what it returns.
type fakeJellyfinTasks struct {
	mu     sync.Mutex
	tasks  []map[string]any
	writes []string
}

func (f *fakeJellyfinTasks) client(t *testing.T) *jellyfinclient.Client {
	t.Helper()
	srv := testnet.Serve(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method == http.MethodGet && r.URL.Path == "/ScheduledTasks" {
			_ = json.NewEncoder(w).Encode(f.tasks)
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/ScheduledTasks/"), "/Triggers")
		for _, task := range f.tasks {
			if task["Id"] == id {
				var triggers []any
				require.NoError(t, json.NewDecoder(r.Body).Decode(&triggers))
				task["Triggers"] = triggers
				f.writes = append(f.writes, task["Key"].(string))
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	hc, err := engine.NewHTTPClient(srv.URL, engine.AuthNone, engine.WithAppLabel("jellyfin"))
	require.NoError(t, err)
	return jellyfinclient.NewClient(hc)
}

func TestJellyfinTriggersAreInTicks(t *testing.T) {
	assert.Equal(t, []map[string]any{
		{"Type": "DailyTrigger", "TimeOfDayTicks": int64(3*60+30) * ticksPerMinute},
		{"Type": "WeeklyTrigger", "TimeOfDayTicks": int64(0), "DayOfWeek": "Sunday", "MaxRuntimeTicks": int64(2*60) * ticksPerMinute},
		{"Type": "IntervalTrigger", "IntervalTicks": int64(720) * ticksPerMinute},
		{"Type": "StartupTrigger"},
	}, jellyfinTriggers([]mediaserversv1alpha1.JellyfinTaskTrigger{
		{Type: "DailyTrigger", TimeOfDay: "03:30"},
		{Type: "WeeklyTrigger", TimeOfDay: "00:00", DayOfWeek: "Sunday", MaxRuntimeHours: 2},
		{Type: "IntervalTrigger", IntervalMinutes: 720},
		{Type: "StartupTrigger"},
	}))
}

func TestJellyfinTasksReplaceOnlyTheTriggersThatDiffer(t *testing.T) {
	app := &fakeJellyfinTasks{tasks: []map[string]any{
		{"Id": "a1", "Key": "RefreshLibrary", "Name": "Scan Media Library",
			"Triggers": []any{map[string]any{"Type": "IntervalTrigger", "IntervalTicks": float64(720 * ticksPerMinute)}}},
		{"Id": "b2", "Key": "RefreshChapterImages", "Name": "Extract Chapter Images",
			"Triggers": []any{map[string]any{"Type": "DailyTrigger", "TimeOfDayTicks": float64(120 * ticksPerMinute), "MaxRuntimeTicks": float64(240 * ticksPerMinute)}}},
	}}
	jf := app.client(t)
	tasks := []mediaserversv1alpha1.JellyfinScheduledTask{
		{Key: "RefreshLibrary", Triggers: []mediaserversv1alpha1.JellyfinTaskTrigger{{Type: "IntervalTrigger", IntervalMinutes: 720}}},
		{Key: "RefreshChapterImages", Triggers: []mediaserversv1alpha1.JellyfinTaskTrigger{}},
		{Key: "NoSuchTask", Triggers: []mediaserversv1alpha1.JellyfinTaskTrigger{{Type: "StartupTrigger"}}},
	}

	problems := reconcileJellyfinTasks(context.Background(), jf, tasks, &engine.Writes{App: "jellyfin"})

	assert.Equal(t, []string{"RefreshChapterImages"}, app.writes, "only the task whose triggers differ is written")
	assert.Empty(t, app.tasks[1]["Triggers"], "an empty list leaves the task to run by hand")
	require.Len(t, problems, 1)
	assert.Equal(t, "scheduledTask(NoSuchTask): Jellyfin has no such task; it has RefreshChapterImages, RefreshLibrary", problems[0])

	reconcileJellyfinTasks(context.Background(), jf, tasks[:2], &engine.Writes{App: "jellyfin"})
	assert.Len(t, app.writes, 1, "second pass must not write")
}

func TestJellyfinTasksObserveRecordsDifferencesWithoutWriting(t *testing.T) {
	app := &fakeJellyfinTasks{tasks: []map[string]any{
		{"Id": "a1", "Key": "RefreshLibrary", "Triggers": []any{}},
	}}
	w := &engine.Writes{App: "jellyfin", Observe: true}

	problems := reconcileJellyfinTasks(context.Background(), app.client(t), []mediaserversv1alpha1.JellyfinScheduledTask{
		{Key: "RefreshLibrary", Triggers: []mediaserversv1alpha1.JellyfinTaskTrigger{{Type: "StartupTrigger"}}},
	}, w)

	assert.Empty(t, problems)
	assert.Empty(t, app.writes)
	assert.Equal(t, []string{"scheduledTasks(RefreshLibrary)"}, w.Drift())
}
