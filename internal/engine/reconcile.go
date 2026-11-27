package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"slices"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// logAPIErrorDetail logs the full API response body at debug level if the error is an APIError.
func logAPIErrorDetail(ctx context.Context, err error) {
	if apiErr, ok := errors.AsType[*APIError](err); ok {
		log.FromContext(ctx).V(1).Info("API error detail", "response", apiErr.DetailedMessage())
	}
}

// DefaultMaxPruneCount is the maximum number of resources that can be pruned
// in a single reconciliation pass per resource type. If more resources would be
// pruned, the operation is refused as a safety measure.
const DefaultMaxPruneCount = 25

// PrunedResource describes a resource that was deleted during pruning.
type PrunedResource struct {
	Type string
	Name string
	ID   int
}

// ReconcileResult holds the outcome of a reconciliation pass.
type ReconcileResult struct {
	Synced []string
	Errors []string
	// Drift lists what observe mode found differing and left alone.
	Drift []string
}

// Success returns true if no errors occurred.
func (r *ReconcileResult) Success() bool {
	return len(r.Errors) == 0
}

// Message returns a human-readable summary including what synced and what failed.
func (r *ReconcileResult) Message() string {
	if r.Success() {
		if len(r.Synced) > 0 {
			return fmt.Sprintf("synced: %v", r.Synced)
		}
		return "all configuration sections synced"
	}
	msg := fmt.Sprintf("errors: %v", r.Errors)
	if len(r.Synced) > 0 {
		msg = fmt.Sprintf("synced: %v; %s", r.Synced, msg)
	}
	return msg
}

// ReconcileApp brings an app's settings and list resources in line through
// pass, and returns what failed. sections maps setting endpoint names to their
// desired state; resources maps resource endpoint names to the items declared.
func ReconcileApp(ctx context.Context, client *HTTPClient, def AppDefinition, sections map[string]any, resources map[string][]map[string]any, pass *Pass) []string {
	logger := log.FromContext(ctx)
	var problems []string
	for _, s := range def.Settings {
		desired, ok := sections[s.Name]
		if !ok || isNilInterface(desired) {
			continue
		}
		drift := len(pass.Drift())
		if err := reconcileSetting(ctx, client, pass, s, desired); err != nil {
			logger.Error(err, "failed to reconcile setting", "section", s.Name)
			logAPIErrorDetail(ctx, err)
			problems = append(problems, fmt.Sprintf("%s: %v", s.Name, err))
			continue
		}
		if len(pass.Drift()) == drift {
			pass.Synced(s.Name)
		}
	}

	for _, r := range def.Resources {
		items := resources[r.Name]
		if len(items) == 0 {
			continue
		}
		existing, err := client.GetJSONList(ctx, r.Path)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s: listing: %v", r.Name, err))
			continue
		}
		failed := false
		for _, item := range items {
			key, drift := r.resourceKey(item), len(pass.Drift())
			if err := reconcileItem(ctx, client, pass, r, &existing, item); err != nil {
				failed = true
				logger.Error(err, "failed to reconcile resource", "type", r.Name, "match", key)
				logAPIErrorDetail(ctx, err)
				problems = append(problems, fmt.Sprintf("%s(%v): %v", r.Name, key, err))
				continue
			}
			if len(pass.Drift()) == drift {
				pass.Synced(fmt.Sprintf("%s(%v)", r.Name, key))
			}
		}
		// Any failed apply for this type blocks pruning until a later pass
		// succeeds.
		if !pass.PruneEnabled || !r.Prunable || failed {
			continue
		}
		if err := pruneEndpoint(ctx, client, pass, r, items, existing); err != nil {
			logger.Error(err, "failed to prune resources", "type", r.Name)
			logAPIErrorDetail(ctx, err)
			problems = append(problems, fmt.Sprintf("%s(prune): %v", r.Name, err))
		}
	}
	return problems
}

// pruneEndpoint deletes the items of r the operator created that items no
// longer declares. It reads the endpoint again before deleting, so an item
// renamed in the app since the pass listed it is never deleted under its old
// name.
func pruneEndpoint(ctx context.Context, client *HTTPClient, pass *Pass, r ResourceEndpoint, declaredItems, existing []map[string]any) error {
	declared := make([]string, 0, len(declaredItems))
	for _, item := range declaredItems {
		if key := r.resourceKey(item); key != "" {
			declared = append(declared, key)
		}
	}
	items := listItems(client.AppLabel(), r.Name, existing, r.resourceKey)
	if len(pass.Owned.Candidates(r.Name, declared, items.Names)) == 0 {
		return nil
	}
	if !pass.Observe {
		latest, err := client.GetJSONList(ctx, r.Path)
		if err != nil {
			return fmt.Errorf("listing: %w", err)
		}
		items = listItems(client.AppLabel(), r.Name, latest, r.resourceKey)
	}
	return pass.PruneItems(r.Name, declared, items, func(id int) error {
		log.FromContext(ctx).Info("pruning unmanaged resource", "type", r.Name, "id", id)
		return client.Delete(ctx, fmt.Sprintf("%s/%d", r.Path, id))
	})
}

// reconcileSetting brings a singleton config endpoint in line: GET, merge, PUT.
func reconcileSetting(ctx context.Context, client *HTTPClient, pass *Pass, s SettingEndpoint, desired any) error {
	current, err := client.GetJSON(ctx, s.Path)
	if err != nil {
		return fmt.Errorf("getting current: %w", err)
	}
	id, ok := current["id"].(float64)
	if !ok {
		return fmt.Errorf("config at %s has no id", s.Path)
	}
	out, err := reconciler.MergeDesired(current, desired)
	if err != nil {
		return fmt.Errorf("merging: %w", err)
	}
	key := client.secretKey("setting", s.Path)
	if !out.Changed && !pass.SecretsDiffer(key, out.SecretDigest) {
		return nil
	}
	return pass.Apply("settings", s.Name, func() error {
		if err := client.PutJSON(ctx, fmt.Sprintf("%s/%d", s.Path, int(id)), out.Merged); err != nil {
			return err
		}
		pass.RecordSecrets(key, out.SecretDigest)
		return nil
	})
}

// reconcileItem brings one declared item in line with the endpoint's items in
// existing, creating it if the app has none. A created item is added to
// existing, so later items in the pass see it.
func reconcileItem(ctx context.Context, client *HTTPClient, pass *Pass, endpoint ResourceEndpoint, existing *[]map[string]any, desired map[string]any) error {
	desiredMatch := endpoint.resourceKey(desired)
	if desiredMatch == "" {
		return fmt.Errorf("resource has no identity")
	}
	key := client.secretKey(endpoint.Name, desiredMatch)
	current, fresh := find(*existing, endpoint, desiredMatch), false
	if current == nil && !pass.Observe && endpoint.Policy != UpdateOnly {
		// Another writer may have created the item since the pass listed the
		// endpoint, and some apps accept a second item with the same name.
		latest, err := client.GetJSONList(ctx, endpoint.Path)
		if err != nil {
			return fmt.Errorf("re-reading: %w", err)
		}
		*existing, fresh = latest, true
		current = find(latest, endpoint, desiredMatch)
	}
	if current != nil {
		if endpoint.Policy == CreateOnly {
			return nil
		}
		update, err := planUpdate(current, desired, endpoint, key, pass)
		if err != nil || update == nil {
			return err
		}
		if !pass.Observe && !fresh {
			// The list is from the start of the pass. Merge over the item as it
			// is now, so a change made in the app since isn't written back.
			latest, err := client.GetJSONList(ctx, endpoint.Path)
			if err != nil {
				return fmt.Errorf("re-reading: %w", err)
			}
			*existing = latest
			if current = find(latest, endpoint, desiredMatch); current == nil {
				return fmt.Errorf("the app no longer has %s %q", endpoint.Name, desiredMatch)
			}
			if update, err = planUpdate(current, desired, endpoint, key, pass); err != nil || update == nil {
				return err
			}
		}
		return pass.Apply(endpoint.Name, desiredMatch, func() error {
			if err := client.PutJSON(ctx, fmt.Sprintf("%s/%d", endpoint.Path, update.id), update.Merged); err != nil {
				return err
			}
			pass.RecordSecrets(key, update.SecretDigest)
			return nil
		})
	}

	if endpoint.Policy == UpdateOnly {
		return fmt.Errorf("the app has no such item, and %s can't be created", endpoint.Name)
	}
	create := desired
	if len(endpoint.CreateDefaults) > 0 {
		var err error
		if create, _, err = reconciler.MergeDesiredOverCurrent(endpoint.CreateDefaults, desired); err != nil {
			return err
		}
	}
	return pass.Apply(endpoint.Name, desiredMatch, func() error {
		body, err := client.Do(ctx, http.MethodPost, endpoint.Path, create)
		if err != nil {
			return err
		}
		pass.Owned.Created(endpoint.Name, desiredMatch)
		// The created resource comes back with its secrets masked, like any
		// later read. Remember what was written now, or the next pass rewrites
		// it once.
		var stored map[string]any
		if json.Unmarshal(body, &stored) == nil {
			*existing = append(*existing, stored)
			if out, err := reconciler.MergeDesired(stored, desired); err == nil {
				pass.RecordSecrets(key, out.SecretDigest)
			}
		}
		return nil
	})
}

// itemUpdate is the body to PUT to an item to bring it in line.
type itemUpdate struct {
	reconciler.MergeOutcome
	id int
}

// planUpdate merges desired over current and returns the update to send, or
// nil when the item already matches.
func planUpdate(current, desired map[string]any, endpoint ResourceEndpoint, key string, pass *Pass) (*itemUpdate, error) {
	id, ok := current["id"].(float64)
	if !ok {
		return nil, fmt.Errorf("resource %q has no id", endpoint.resourceKey(current))
	}
	trimmedCurrent, trimmed, err := withoutUndeclared(current, desired, endpoint.ExactLists)
	if err != nil {
		return nil, err
	}
	out, err := reconciler.MergeDesired(trimmedCurrent, desired)
	if err != nil {
		return nil, err
	}
	if !trimmed && !out.Changed && !pass.SecretsDiffer(key, out.SecretDigest) {
		return nil, nil
	}
	return &itemUpdate{MergeOutcome: out, id: int(id)}, nil
}

// find returns the item of endpoint whose key is key, or nil.
func find(items []map[string]any, endpoint ResourceEndpoint, key string) map[string]any {
	for _, item := range items {
		if endpoint.resourceKey(item) == key {
			return item
		}
	}
	return nil
}

// withoutUndeclared drops the entries of each exact list in current whose name
// desired doesn't declare, and reports whether it dropped any.
func withoutUndeclared(current, desired map[string]any, lists []string) (map[string]any, bool, error) {
	out, trimmed := current, false
	for _, key := range lists {
		raw, ok := desired[key]
		if !ok {
			continue
		}
		data, err := json.Marshal(raw)
		if err != nil {
			return nil, false, fmt.Errorf("reading declared %s: %w", key, err)
		}
		var declared []struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &declared); err != nil {
			return nil, false, fmt.Errorf("reading declared %s: %w", key, err)
		}
		names := make(map[string]bool, len(declared))
		for _, d := range declared {
			names[d.Name] = true
		}
		existing, _ := current[key].([]any)
		kept := slices.DeleteFunc(slices.Clone(existing), func(e any) bool {
			name, _ := e.(map[string]any)["name"].(string)
			return !names[name]
		})
		if len(kept) == len(existing) {
			continue
		}
		if !trimmed {
			out = maps.Clone(current)
		}
		out[key] = kept
		trimmed = true
	}
	return out, trimmed, nil
}

// secretKey names an item's last-written secrets within its resource's
// records. Quoted components keep arbitrary names from colliding with
// separators.
func (c *HTTPClient) secretKey(resourceType, name string) string {
	return fmt.Sprintf("%q/%q/%q/%q", c.baseURL, c.AppLabel(), resourceType, name)
}

func isNilInterface(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() == reflect.Pointer && rv.IsNil()
}

// KeptResource is an owned resource the app refused to delete.
type KeptResource struct {
	PrunedResource
	Reason string
}

// DeleteManagedResources deletes the items owned records, for deletionPolicy
// delete. Reverse declaration order removes dependents (for example profiles)
// before resources they reference (custom formats). Settings, root folders and
// tags are kept. An item the app refuses to delete, such as a quality profile a
// root folder still uses, is kept and comes back in a *KeptError; any other
// failure stops the cleanup so it can be retried.
func DeleteManagedResources(ctx context.Context, client *HTTPClient, def AppDefinition, owned *Ownership) error {
	var kept []error
	for _, endpoint := range slices.Backward(def.Resources) {
		if !endpoint.Prunable || !owned.Owns(endpoint.Name) {
			continue
		}
		existing, err := client.GetJSONList(ctx, endpoint.Path)
		if err != nil {
			return errors.Join(append(kept, fmt.Errorf("cleaning up %s: listing: %w", endpoint.Name, err))...)
		}
		items := listItems(client.AppLabel(), endpoint.Name, existing, endpoint.resourceKey)
		err = owned.Cleanup(endpoint.Name, items.Names, func(name string) error {
			id, err := items.ID(name)
			if err != nil {
				return err
			}
			if err := client.Delete(ctx, fmt.Sprintf("%s/%d", endpoint.Path, id)); err != nil && !isNotFound(err) {
				return err
			}
			return nil
		})
		refusals, failure := SplitKept(err)
		for _, refusal := range refusals {
			kept = append(kept, refusal)
		}
		if failure != nil {
			return errors.Join(append(kept, fmt.Errorf("cleaning up %s: %w", endpoint.Name, failure))...)
		}
	}
	return errors.Join(kept...)
}

// Refusal reports whether err is the app declining a request it understood,
// with the reason it gave. Authentication failures, rate limits and timeouts
// are not refusals; they may succeed on a retry.
func Refusal(err error) (string, bool) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode < 400 || apiErr.StatusCode >= 500 {
		return "", false
	}
	switch apiErr.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusRequestTimeout, http.StatusTooManyRequests:
		return "", false
	}
	var body struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(apiErr.Body, &body) == nil && body.Message != "" {
		return reconciler.Redact(body.Message), true
	}
	return fmt.Sprintf("status %d", apiErr.StatusCode), true
}

func isNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
