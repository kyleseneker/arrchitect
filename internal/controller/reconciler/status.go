package reconciler

import (
	"context"
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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/events"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// Result is what a pass did: the items it brought in line, what failed, and
// what observe mode found differing.
type Result struct {
	Synced []string
	Errors []error
	Drift  []string
}

// ReportResult records a pass's result in obj's status, and returns how soon
// a failure asks for the next pass, or zero for the resource's interval.
func ReportResult(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, observe bool, result Result) time.Duration {
	reason, after := syncFailure(result.Errors)
	failures := make([]string, 0, len(result.Errors))
	for _, err := range result.Errors {
		failures = append(failures, err.Error())
	}
	switch {
	case len(failures) > 0 && observe:
		UpdateStatus(ctx, c, recorder, obj, false, reason, "observation failed: "+listed(failures, "; "))
	case len(failures) > 0:
		UpdateStatus(ctx, c, recorder, obj, false, reason, listed(failures, "; "))
	case observe && len(result.Drift) > 0:
		UpdateStatus(ctx, c, recorder, obj, false, ReasonDriftDetected,
			"differs: "+listed(result.Drift, ", ")+"; observe policy prevents application writes")
	case observe:
		UpdateStatus(ctx, c, recorder, obj, true, ReasonObserved, "observable configuration matches; no application writes performed")
	case len(result.Synced) > 0:
		UpdateStatus(ctx, c, recorder, obj, true, ReasonSynced, "synced: "+listed(result.Synced, ", "))
	default:
		UpdateStatus(ctx, c, recorder, obj, true, ReasonSynced, "all configuration sections synced")
	}
	if after == 0 {
		return 0
	}
	return wait.Jitter(after, jitter)
}

// maxListed caps how many items a status message names, which also keeps a
// pass with many failures inside a condition message's 32 KiB.
const maxListed = 10

// listed joins the distinct items with sep, naming at most maxListed.
func listed(items []string, sep string) string {
	var unique []string
	for _, item := range items {
		if !slices.Contains(unique, item) {
			unique = append(unique, item)
		}
	}
	if len(unique) <= maxListed {
		return strings.Join(unique, sep)
	}
	return fmt.Sprintf("%s and %d more", strings.Join(unique[:maxListed], sep), len(unique)-maxListed)
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
	ReasonInvalidConfig: true,
	ReasonNameConflict:  true,
	ReasonUnauthorized:  true,
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
	if previous := meta.FindStatusCondition(*conditions, ConditionReady); previous != nil {
		before = previous.DeepCopy()
	}
	SetCondition(conditions, generation, ConditionSynced, status, reason, message)
	SetCondition(conditions, generation, ConditionReady, status, reason, message)
	if !synced && stalledReasons[reason] {
		SetCondition(conditions, generation, ConditionStalled, metav1.ConditionTrue, reason, message)
	} else {
		meta.RemoveStatusCondition(conditions, ConditionStalled)
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
	ReasonSynced:            "The app matches the resource again.",
	ReasonObserved:          "The app matches the resource again.",
	ReasonSyncFailed:        "The app refused some settings. The Ready condition says which.",
	ReasonDriftDetected:     "The app differs from the resource. The Ready condition says where.",
	ReasonAppUnreachable:    "The app didn't respond.",
	ReasonUnauthorized:      "The app refused the credentials.",
	ReasonSecretNotFound:    "A Secret the resource uses is missing. The Ready condition names it.",
	ReasonReferenceNotFound: "A resource this one refers to is missing. The Ready condition names it.",
	ReasonInvalidConfig:     "The resource asks for something the operator can't do. The Ready condition says what.",
	ReasonNameConflict:      "Another resource already configures this item.",
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
// deleted resource clears, and the state kept for it between passes.
func IgnoreDeleted(obj ConfigResource, key types.NamespacedName, err error) error {
	if apierrors.IsNotFound(err) {
		metrics.ConfigSynced.DeleteLabelValues(reflect.TypeOf(obj).Elem().Name(), key.Namespace, key.Name)
		metrics.ManagedResources.DeletePartialMatch(resourceLabels(key.Namespace, key.Name, obj))
		obj.SetNamespace(key.Namespace)
		obj.SetName(key.Name)
		forget(obj)
		secrets.ForgetSecrets(ownerPrefix(obj))
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
		recorder.Eventf(obj, nil, corev1.EventTypeWarning, ReasonResourcePruned, "Prune",
			"Deleted %s %q%s, which the operator created", p.Type, p.Name, id)
	}
}
