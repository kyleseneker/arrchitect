package common

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/reconciler"
)

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

const DriftPolicyObserve = "observe"

// ReconcileInterval returns the configured interval or the default.
func ReconcileInterval(rc *commonv1alpha1.ReconcileConfig) time.Duration {
	if rc != nil && rc.IntervalSeconds != 0 {
		return time.Duration(rc.IntervalSeconds) * time.Second
	}
	return engine.DefaultReconcileInterval
}

// PruneEnabled returns whether resource pruning is enabled.
func PruneEnabled(rc *commonv1alpha1.ReconcileConfig) bool {
	return rc != nil && rc.Prune != nil && *rc.Prune
}

// ObserveOnly reports whether drift should be recorded without writing.
func ObserveOnly(rc *commonv1alpha1.ReconcileConfig) bool {
	return rc != nil && rc.DriftPolicy == DriftPolicyObserve
}

// Policy renders the reconcile policy for a CR.
func Policy(rc *commonv1alpha1.ReconcileConfig) engine.SyncPolicy {
	return engine.SyncPolicy{Prune: PruneEnabled(rc), Observe: ObserveOnly(rc)}
}

// updateObservationStatus reports comparison results without claiming drift was
// corrected. Read failures take precedence over any differences already found.
func updateObservationStatus(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, drift []string, err error) {
	if err != nil {
		UpdateStatus(ctx, c, recorder, obj, false, engine.ReasonSyncFailed, "observation failed: "+err.Error())
	} else if len(drift) > 0 {
		UpdateStatus(ctx, c, recorder, obj, false, engine.ReasonDriftDetected, "differs: "+driftSummary(drift)+"; observe policy prevents application writes")
	} else {
		UpdateStatus(ctx, c, recorder, obj, true, engine.ReasonObserved, "observable configuration matches; no application writes performed")
	}
}

// ReportResult is UpdateResultStatus for an object whose drift policy is set
// somewhere other than on the object itself.
func ReportResult(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, observe bool, result engine.ReconcileResult) {
	if observe {
		updateObservationStatus(ctx, c, recorder, obj, result.Drift, syncError(result.Errors))
		return
	}
	UpdateStatus(ctx, c, recorder, obj, result.Success(), resultReason(result), result.Message())
}

// maxDriftListed caps how many differences a status message names.
const maxDriftListed = 10

func driftSummary(drift []string) string {
	var unique []string
	for _, d := range drift {
		if !slices.Contains(unique, d) {
			unique = append(unique, d)
		}
	}
	if len(unique) <= maxDriftListed {
		return strings.Join(unique, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(unique[:maxDriftListed], ", "), len(unique)-maxDriftListed)
}

func syncError(syncErrors []string) error {
	if len(syncErrors) == 0 {
		return nil
	}
	return fmt.Errorf("%v", syncErrors)
}

// ResultReason returns the appropriate status reason for a reconcile result.
func resultReason(r engine.ReconcileResult) string {
	if r.Success() {
		return engine.ReasonSynced
	}
	return engine.ReasonSyncFailed
}

// UpdateStatusManaged records the resources the operator now manages, so prune
// can distinguish them from resources created outside the operator.
func UpdateStatusManaged(obj ConfigResource, managed map[string][]string) {
	if managed == nil {
		return
	}
	obj.GetConfigStatus().SetManagedResources(managed)
}

// recordManaged sets the managed_resources series from the ownership obj's
// status records, on every status write, so they survive an operator restart
// while the app is out of reach.
func recordManaged(obj ConfigResource) {
	labels := resourceLabels(obj.GetNamespace(), obj.GetName(), obj)
	metrics.ManagedResources.DeletePartialMatch(labels)
	for resourceType, names := range obj.GetConfigStatus().GetManagedResources() {
		if len(names) > 0 {
			metrics.ManagedResources.WithLabelValues(labels["kind"], labels["namespace"], labels["name"], resourceType).Set(float64(len(names)))
		}
	}
}

// resourceLabels identifies a resource in metrics. TypeMeta is cleared on
// typed objects after a client round-trip, so the Go type names the kind.
func resourceLabels(namespace, name string, obj ConfigResource) prometheus.Labels {
	return prometheus.Labels{"kind": reflect.TypeOf(obj).Elem().Name(), "namespace": namespace, "name": name}
}

// stalledReasons are failures no retry fixes: the spec, a Secret or the app
// has to change first.
var stalledReasons = map[string]bool{
	engine.ReasonInvalidConfig: true,
	engine.ReasonNameConflict:  true,
	engine.ReasonUnauthorized:  true,
}

// UpdateStatus records the outcome of a pass and persists it. Synced says
// whether the app matches the resource. Ready repeats it as the summary that
// GitOps tools read, and Stalled marks a failure no retry fixes. Each change
// of Ready is recorded as an event.
func UpdateStatus(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, synced bool, reason, message string) {
	cs := obj.GetConfigStatus()
	status := metav1.ConditionFalse
	if synced {
		status = metav1.ConditionTrue
		now := metav1.Now()
		cs.LastSyncTime = &now
	}
	generation := obj.GetGeneration()
	conditions := &cs.Conditions
	var before *metav1.Condition
	if previous := meta.FindStatusCondition(*conditions, engine.ConditionReady); previous != nil {
		before = previous.DeepCopy()
	}
	reconciler.SetCondition(conditions, generation, engine.ConditionSynced, status, reason, message)
	reconciler.SetCondition(conditions, generation, engine.ConditionReady, status, reason, message)
	if !synced && stalledReasons[reason] {
		reconciler.SetCondition(conditions, generation, engine.ConditionStalled, metav1.ConditionTrue, reason, message)
	} else {
		meta.RemoveStatusCondition(conditions, engine.ConditionStalled)
	}
	cs.ObservedGeneration = generation
	recordSynced(obj, synced)
	recordManaged(obj)
	if writeStatus(ctx, c, obj) {
		emitTransition(recorder, obj, before, synced, reason)
	}
}

// transitionNotes say in a fixed sentence what each reason means. An event
// can reach readers who can't read the resource, such as an event exporter,
// so the app's own words stay in the Ready condition.
var transitionNotes = map[string]string{
	engine.ReasonSynced:            "The app matches the resource again.",
	engine.ReasonObserved:          "The app matches the resource again.",
	engine.ReasonSyncFailed:        "The app refused some settings. The Ready condition says which.",
	engine.ReasonDriftDetected:     "The app differs from the resource. The Ready condition says where.",
	engine.ReasonAppUnreachable:    "The app didn't respond.",
	engine.ReasonUnauthorized:      "The app refused the credentials.",
	engine.ReasonSecretNotFound:    "A Secret the resource uses is missing. The Ready condition names it.",
	engine.ReasonReferenceNotFound: "A resource this one refers to is missing. The Ready condition names it.",
	engine.ReasonInvalidConfig:     "The resource asks for something the operator can't do. The Ready condition says what.",
	engine.ReasonNameConflict:      "Another resource already configures this item.",
}

// emitTransition records a Warning when Ready turns False or its reason
// changes, and a Normal event when it turns True again.
func emitTransition(recorder events.EventRecorder, obj ConfigResource, before *metav1.Condition, synced bool, reason string) {
	if recorder == nil {
		return
	}
	note, ok := transitionNotes[reason]
	if !ok {
		note = "Ready is now " + reason + "."
	}
	switch {
	case !synced && (before == nil || before.Status != metav1.ConditionFalse || before.Reason != reason):
		recorder.Eventf(obj, nil, corev1.EventTypeWarning, reason, "Reconcile", "%s", note)
	case synced && before != nil && before.Status == metav1.ConditionFalse:
		recorder.Eventf(obj, nil, corev1.EventTypeNormal, reason, "Reconcile", "%s", note)
	}
}

// IgnoreDeleted is client.IgnoreNotFound for a reconcile's first read. A
// resource that is gone also loses its config_synced series, so an alert on a
// deleted resource clears.
func IgnoreDeleted(obj ConfigResource, key types.NamespacedName, err error) error {
	if apierrors.IsNotFound(err) {
		metrics.ConfigSynced.DeleteLabelValues(reflect.TypeOf(obj).Elem().Name(), key.Namespace, key.Name)
		metrics.ManagedResources.DeletePartialMatch(resourceLabels(key.Namespace, key.Name, obj))
		obj.SetNamespace(key.Namespace)
		obj.SetName(key.Name)
		forget(obj)
	}
	return client.IgnoreNotFound(err)
}

func recordSynced(obj ConfigResource, synced bool) {
	v := 0.0
	if synced {
		v = 1.0
	}
	// TypeMeta is cleared on typed objects after a client round-trip, so the
	// concrete Go type is the reliable source for the kind label.
	kind := reflect.TypeOf(obj).Elem().Name()
	metrics.ConfigSynced.WithLabelValues(kind, obj.GetNamespace(), obj.GetName()).Set(v)
}

// statusWriteTimeout bounds a status write, which runs on after its reconcile
// is cancelled.
const statusWriteTimeout = 10 * time.Second

// writeStatus persists obj's status and reports whether it did. Only this
// controller writes the status, so a conflict means only that something else
// changed the object mid-pass, most often a spec edit. Losing the write would
// also lose any ownership this pass recorded in status.managedResources, and a
// resource the operator created but no longer tracks can never be pruned or
// cleaned up. So retry against the latest resourceVersion; the status
// subresource ignores everything but status. The latest version is read past
// the cache, which can still hold the version from before the edit, and the
// write outlives a reconcile cancelled mid-pass.
func writeStatus(ctx context.Context, c client.Client, obj ConfigResource) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusWriteTimeout)
	defer cancel()
	var latestReader client.Reader = c
	if live, ok := c.(interface{ APIReader() client.Reader }); ok {
		latestReader = live.APIReader()
	}
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		err := c.Status().Update(ctx, obj)
		if !apierrors.IsConflict(err) {
			return err
		}
		latest, ok := obj.DeepCopyObject().(client.Object)
		if !ok {
			return err
		}
		if getErr := latestReader.Get(ctx, client.ObjectKeyFromObject(obj), latest); getErr != nil {
			return getErr
		}
		obj.SetResourceVersion(latest.GetResourceVersion())
		return err
	})
	if err != nil && !apierrors.IsNotFound(err) {
		log.FromContext(ctx).Error(err, "failed to update status")
	}
	return err == nil
}

// BoolTo01 converts a bool to "1" or "0" for APIs that use numeric strings (Plex, SABnzbd).
func BoolTo01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// EnqueueForSecret re-reconciles every config of the listed kind, in a changed
// Secret's namespace, that refers to that Secret.
func EnqueueForSecret(c client.Client, list client.ObjectList) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(configsForSecret(c, list))
}

func configsForSecret(c client.Client, list client.ObjectList) handler.MapFunc {
	return secretReferrers(c, nil, list, func(client.Object) []commonv1alpha1.AppRef { return nil })
}

// ResolveConfigFields returns a copy of fields with any valueFrom reference
// resolved into a literal value, so secrets stay out of the CR.
func ResolveConfigFields(ctx context.Context, c client.Reader, namespace, owner string, fields []commonv1alpha1.ConfigField) ([]commonv1alpha1.ConfigField, error) {
	if len(fields) == 0 {
		return fields, nil
	}
	out := make([]commonv1alpha1.ConfigField, len(fields))
	copy(out, fields)
	for i := range out {
		if out[i].ValueFrom == (commonv1alpha1.ValueSource{}) {
			continue
		}
		if out[i].Value != nil {
			return nil, fmt.Errorf("%s field %q: value and valueFrom are mutually exclusive", owner, out[i].Name)
		}
		val, err := reconciler.ResolveSecretKeyRef(ctx, c, namespace, out[i].ValueFrom.SecretKeyRef)
		if err != nil {
			return nil, fmt.Errorf("%s field %q: %w", owner, out[i].Name, err)
		}
		raw, err := json.Marshal(val)
		if err != nil {
			return nil, fmt.Errorf("%s field %q: encoding secret value: %w", owner, out[i].Name, err)
		}
		out[i].Value = &commonv1alpha1.FieldValue{Raw: raw}
		out[i].ValueFrom = commonv1alpha1.ValueSource{}
	}
	return out, nil
}

// FieldValues resolves fields into values by name, reading the ones that come
// from Secrets.
func FieldValues(ctx context.Context, c client.Reader, namespace, owner string, fields []commonv1alpha1.ConfigField) (map[string]any, error) {
	resolved, err := ResolveConfigFields(ctx, c, namespace, owner, fields)
	if err != nil {
		return nil, err
	}
	out := make(map[string]any, len(resolved))
	for _, f := range resolved {
		if f.Value != nil {
			out[f.Name] = f.Value.ToInterface()
		}
	}
	return out, nil
}

// ResolveItemFields returns a copy of items with each item's field Secret
// references resolved. what names the kind of item in errors, and fields
// returns an item's name and its fields.
func ResolveItemFields[T any](
	ctx context.Context, c client.Reader, namespace, what string, items []T,
	fields func(*T) (string, *[]commonv1alpha1.ConfigField),
) ([]T, error) {
	out := slices.Clone(items)
	for i := range out {
		name, f := fields(&out[i])
		resolved, err := ResolveConfigFields(ctx, c, namespace, what+" "+name, *f)
		if err != nil {
			return nil, err
		}
		*f = resolved
	}
	return out, nil
}

// ResolveIndexerSecrets returns indexers with their field secret references resolved.
func ResolveIndexerSecrets(ctx context.Context, c client.Reader, namespace string, in []commonv1alpha1.Indexer) ([]commonv1alpha1.Indexer, error) {
	return ResolveItemFields(ctx, c, namespace, "indexer", in, func(i *commonv1alpha1.Indexer) (string, *[]commonv1alpha1.ConfigField) {
		return i.Name, &i.Fields
	})
}

// ResolveNotificationSecrets returns notifications with their field secret references resolved.
func ResolveNotificationSecrets(ctx context.Context, c client.Reader, namespace string, in []commonv1alpha1.Notification) ([]commonv1alpha1.Notification, error) {
	return ResolveItemFields(ctx, c, namespace, "notification", in, func(n *commonv1alpha1.Notification) (string, *[]commonv1alpha1.ConfigField) {
		return n.Name, &n.Fields
	})
}

// ResolveImportListSecrets returns import lists with their field secret references resolved.
func ResolveImportListSecrets(ctx context.Context, c client.Reader, namespace string, in []commonv1alpha1.ImportList) ([]commonv1alpha1.ImportList, error) {
	return ResolveItemFields(ctx, c, namespace, "import list", in, func(il *commonv1alpha1.ImportList) (string, *[]commonv1alpha1.ConfigField) {
		return il.Name, &il.Fields
	})
}

var secretKeyRefType = reflect.TypeFor[commonv1alpha1.SecretKeyRef]()

// referencesSecret reports whether a config refers to the named Secret
// anywhere. Every Secret reference in the API is a SecretKeyRef, so walking
// the config finds new ones as fields are added rather than relying on a list
// kept by hand.
func referencesSecret(config any, name string) bool {
	return slices.ContainsFunc(SecretKeyRefs(config), func(ref commonv1alpha1.SecretKeyRef) bool { return ref.Name == name })
}

// SecretKeyRefs lists every SecretKeyRef in a config.
func SecretKeyRefs(config any) []commonv1alpha1.SecretKeyRef {
	var refs []commonv1alpha1.SecretKeyRef
	collectSecretKeyRefs(reflect.ValueOf(config), &refs)
	return refs
}

func collectSecretKeyRefs(v reflect.Value, refs *[]commonv1alpha1.SecretKeyRef) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			collectSecretKeyRefs(v.Elem(), refs)
		}
	case reflect.Struct:
		if v.Type() == secretKeyRefType {
			if !v.IsZero() {
				*refs = append(*refs, v.Interface().(commonv1alpha1.SecretKeyRef))
			}
			return
		}
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				collectSecretKeyRefs(v.Field(i), refs)
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		for i := range v.Len() {
			collectSecretKeyRefs(v.Index(i), refs)
		}
	case reflect.Map:
		keys := v.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int { return strings.Compare(a.String(), b.String()) })
		for _, k := range keys {
			collectSecretKeyRefs(v.MapIndex(k), refs)
		}
	}
}

// EmitPruneEvents emits a Warning event for each resource the operator deleted
// from the app, by prune or by deletionPolicy delete. Either only ever deletes
// resources the operator created.
func EmitPruneEvents(recorder events.EventRecorder, obj client.Object, pruned []engine.PrunedResource) {
	if recorder == nil {
		return
	}
	for _, p := range pruned {
		id := ""
		if p.ID != 0 {
			id = fmt.Sprintf(" (id=%d)", p.ID)
		}
		recorder.Eventf(obj, nil, corev1.EventTypeWarning, engine.ReasonResourcePruned, "Prune",
			"Deleted %s %q%s, which the operator created", p.Type, p.Name, id)
	}
}

// ConfigChangedPredicate ignores status-only updates generated by reconciliation.
// Deletion and finalizer changes must still enqueue even without a generation
// change. Apply this to the CR watch only, never to Secret watches.
func ConfigChangedPredicate() predicate.Predicate {
	return predicate.Funcs{UpdateFunc: func(e event.UpdateEvent) bool {
		if e.ObjectOld == nil || e.ObjectNew == nil {
			return false
		}
		return e.ObjectOld.GetGeneration() != e.ObjectNew.GetGeneration() ||
			!e.ObjectOld.GetDeletionTimestamp().Equal(e.ObjectNew.GetDeletionTimestamp()) ||
			!slices.Equal(e.ObjectOld.GetFinalizers(), e.ObjectNew.GetFinalizers()) ||
			!reflect.DeepEqual(e.ObjectOld.GetAnnotations(), e.ObjectNew.GetAnnotations())
	}}
}
