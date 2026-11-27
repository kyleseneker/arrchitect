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
	"k8s.io/client-go/tools/events"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/metrics"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch

// state is how a pass ended, as the Synced condition says it.
type state struct {
	synced  bool
	reason  string
	message string
	stalled bool
}

// failedState is the state of a pass that failed with errs, which takes its
// reason from out. An observe pass that failed says so.
func failedState(out outcome, errs []error, observing bool) state {
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		messages = append(messages, err.Error())
	}
	message := listed(messages, "; ")
	if observing {
		message = "observation failed: " + message
	}
	return state{reason: out.reason, message: message, stalled: out.stalled}
}

// passState is the state of a pass that ran without failing: the items it
// brought in line, or what observe mode found differing.
func passState(observe bool, synced, drift []string) state {
	switch {
	case observe && len(drift) > 0:
		return state{reason: ReasonDriftDetected,
			message: "differs: " + listed(drift, ", ") + "; observe policy prevents application writes"}
	case observe:
		return state{synced: true, reason: ReasonObserved, message: "observable configuration matches; no application writes performed"}
	case len(synced) > 0:
		return state{synced: true, reason: ReasonSynced, message: "synced: " + listed(synced, ", ")}
	default:
		return state{synced: true, reason: ReasonSynced, message: "all configuration sections synced"}
	}
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

// record writes st to obj's status and records a change of Ready as an event.
// A resource deleted mid-pass gets neither.
// Synced says whether the app matches the resource. Ready repeats it as the
// summary GitOps tools read, and Stalled marks a failure no retry fixes. base
// is obj as the pass found it, which the status is patched over.
func record(ctx context.Context, c client.Client, recorder events.EventRecorder, obj ConfigResource, base client.Object, st state) error {
	cs := obj.GetConfigStatus()
	status := metav1.ConditionFalse
	if st.synced {
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
	setCondition(conditions, generation, ConditionSynced, status, st.reason, st.message)
	setCondition(conditions, generation, ConditionReady, status, st.reason, st.message)
	if st.stalled {
		setCondition(conditions, generation, ConditionStalled, metav1.ConditionTrue, st.reason, st.message)
	} else {
		meta.RemoveStatusCondition(conditions, ConditionStalled)
	}
	cs.ObservedGeneration = generation
	recordSynced(obj, st.synced)
	recordManaged(obj)
	err := writeStatus(ctx, c, obj, base)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	emitTransition(recorder, obj, before, st.synced, st.reason)
	return nil
}

// statusWriteTimeout bounds a status write, which runs on after its reconcile
// is cancelled.
const statusWriteTimeout = 10 * time.Second

// writeStatus patches obj's status over base. A merge patch carries no
// resourceVersion, so a spec edit mid-pass can't conflict with it. The write
// outlives a reconcile cancelled mid-pass, which would otherwise lose the
// ownership the pass recorded.
func writeStatus(ctx context.Context, c client.Client, obj, base client.Object) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), statusWriteTimeout)
	defer cancel()
	return c.Status().Patch(ctx, obj, client.MergeFrom(base))
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

func recordSynced(obj ConfigResource, synced bool) {
	v := 0.0
	if synced {
		v = 1.0
	}
	metrics.ConfigSynced.WithLabelValues(kindOf(obj), obj.GetNamespace(), obj.GetName()).Set(v)
}

// resourceLabels identifies a resource in metrics.
func resourceLabels(namespace, name string, obj ConfigResource) prometheus.Labels {
	return prometheus.Labels{"kind": kindOf(obj), "namespace": namespace, "name": name}
}

// kindOf names obj's kind. TypeMeta is cleared on typed objects after a client
// round-trip, so the Go type names it.
func kindOf(obj client.Object) string {
	return reflect.TypeOf(obj).Elem().Name()
}

// forgetDeleted is client.IgnoreNotFound for a reconcile's first read. A
// resource that is gone also loses its metric series, so an alert on a deleted
// resource clears, and the state kept for it between passes.
func forgetDeleted(obj ConfigResource, key types.NamespacedName, err error) error {
	if apierrors.IsNotFound(err) {
		metrics.ConfigSynced.DeleteLabelValues(kindOf(obj), key.Namespace, key.Name)
		metrics.ManagedResources.DeletePartialMatch(resourceLabels(key.Namespace, key.Name, obj))
		obj.SetNamespace(key.Namespace)
		obj.SetName(key.Name)
		forgetOwnership(obj)
		secrets.ForgetSecrets(ownerPrefix(obj))
	}
	return client.IgnoreNotFound(err)
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

// emitPruneEvents emits a Warning event for each resource the operator
// deleted from the app, by prune or by deletionPolicy delete. Either only ever
// deletes resources the operator created.
func emitPruneEvents(recorder events.EventRecorder, obj client.Object, pruned []engine.PrunedResource) {
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
