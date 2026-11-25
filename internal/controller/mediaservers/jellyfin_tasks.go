package mediaservers

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	jellyfinclient "github.com/kyleseneker/arrchitect/internal/client/jellyfin"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// ticksPerMinute converts minutes to the 100-nanosecond ticks Jellyfin keeps
// trigger times in.
const ticksPerMinute = 600_000_000

// jellyfinTaskChange is a task whose triggers differ from the declared ones.
type jellyfinTaskChange struct {
	key, id  string
	triggers []map[string]any
}

// jellyfinTaskChanges finds the declared tasks whose triggers differ from
// Jellyfin's, and reports a task Jellyfin doesn't have.
func jellyfinTaskChanges(
	ctx context.Context, jf *jellyfinclient.Client, tasks []mediaserversv1alpha1.JellyfinScheduledTask,
) ([]jellyfinTaskChange, []string, error) {
	if len(tasks) == 0 {
		return nil, nil, nil
	}
	current, err := jf.ScheduledTasks(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("listing scheduled tasks: %w", err)
	}
	byKey := make(map[string]map[string]any, len(current))
	for _, task := range current {
		if key, ok := task["Key"].(string); ok {
			byKey[key] = task
		}
	}
	var changes []jellyfinTaskChange
	var problems []string
	for _, task := range tasks {
		existing, ok := byKey[task.Key]
		if !ok {
			problems = append(problems, fmt.Sprintf("scheduledTask(%s): Jellyfin has no such task; it has %s",
				task.Key, strings.Join(slices.Sorted(maps.Keys(byKey)), ", ")))
			continue
		}
		desired := jellyfinTriggers(task.Triggers)
		if reflect.DeepEqual(normalizedTriggers(existing["Triggers"]), normalizedTriggers(desired)) {
			continue
		}
		id, _ := existing["Id"].(string)
		changes = append(changes, jellyfinTaskChange{key: task.Key, id: id, triggers: desired})
	}
	return changes, problems, nil
}

// reconcileJellyfinTasks replaces the triggers of the declared tasks that
// differ, or in observe mode records them as differences.
func reconcileJellyfinTasks(
	ctx context.Context, jf *jellyfinclient.Client, tasks []mediaserversv1alpha1.JellyfinScheduledTask, w *engine.Writes,
) []string {
	changes, problems, err := jellyfinTaskChanges(ctx, jf, tasks)
	if err != nil {
		return []string{err.Error()}
	}
	for _, change := range changes {
		err := w.Apply("scheduledTasks", change.key, func() error {
			return jf.SetTaskTriggers(ctx, change.id, change.triggers)
		})
		if err != nil {
			problems = append(problems, fmt.Sprintf("scheduledTask(%s): %v", change.key, err))
		}
	}
	return problems
}

// jellyfinTriggers renders triggers as Jellyfin stores them, with times in
// ticks.
func jellyfinTriggers(triggers []mediaserversv1alpha1.JellyfinTaskTrigger) []map[string]any {
	out := make([]map[string]any, 0, len(triggers))
	for _, t := range triggers {
		trigger := map[string]any{"Type": t.Type}
		if hours, minutes, ok := strings.Cut(t.TimeOfDay, ":"); ok {
			h, _ := strconv.Atoi(hours)
			m, _ := strconv.Atoi(minutes)
			trigger["TimeOfDayTicks"] = int64(h*60+m) * ticksPerMinute
		}
		if t.DayOfWeek != "" {
			trigger["DayOfWeek"] = t.DayOfWeek
		}
		if t.IntervalMinutes != 0 {
			trigger["IntervalTicks"] = int64(t.IntervalMinutes) * ticksPerMinute
		}
		if t.MaxRuntimeHours != 0 {
			trigger["MaxRuntimeTicks"] = int64(t.MaxRuntimeHours) * 60 * ticksPerMinute
		}
		out = append(out, trigger)
	}
	return out
}

// normalizedTriggers keeps each trigger's set fields, with numbers as JSON
// decodes them, so Jellyfin's triggers compare with declared ones.
func normalizedTriggers(v any) []map[string]any {
	var list []map[string]any
	switch triggers := v.(type) {
	case []any:
		for _, t := range triggers {
			if m, ok := t.(map[string]any); ok {
				list = append(list, m)
			}
		}
	case []map[string]any:
		list = triggers
	}
	out := make([]map[string]any, 0, len(list))
	for _, t := range list {
		trigger := map[string]any{}
		for _, key := range []string{"Type", "TimeOfDayTicks", "IntervalTicks", "DayOfWeek", "MaxRuntimeTicks"} {
			switch value := t[key].(type) {
			case nil:
			case int64:
				trigger[key] = float64(value)
			default:
				trigger[key] = value
			}
		}
		out = append(out, trigger)
	}
	return out
}
