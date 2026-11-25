package pvr

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	pvrclient "github.com/kyleseneker/arrchitect/internal/client/pvr"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// instanceRefIndex indexes object resources by the instance they name, so a
// change to an instance requeues its objects.
const instanceRefIndex = ".spec.instanceRef"

// pvrInstance is what an object takes from the instance it belongs to.
type pvrInstance struct {
	connection commonv1alpha1.AppConnection
	policy     *commonv1alpha1.ReconcileConfig
}

// pvrObject is one object in an app, such as an indexer, reconciled through
// the instance it names.
type pvrObject struct {
	app pvrclient.App
	obj ctrlcommon.ConfigResource
	// instanceKind and instanceName name the instance for status messages.
	instanceKind, instanceName string
	instance                   func(context.Context) (pvrInstance, error)
	// path lists the app's objects of this kind, to find this one's ID by name.
	path, name string
	// list and claim find other resources of this kind naming the same object:
	// claim returns the instance and object a resource of this kind names.
	list  client.ObjectList
	claim func(client.Object) (instance, name string)
	// options returns the object as the engine applies it, with its Secrets
	// resolved.
	options func(context.Context) (pvrclient.Options, error)
	setID   func(int32)
}

func reconcilePVRObject(ctx context.Context, c client.Client, recorder events.EventRecorder, o pvrObject) (ctrl.Result, error) {
	obj := o.obj
	instance, err := o.instance(ctx)
	missing := apierrors.IsNotFound(err)
	if err != nil && !missing {
		return ctrl.Result{}, err
	}

	cleanup := func(ctx context.Context) error {
		if missing {
			recorder.Eventf(obj, nil, corev1.EventTypeWarning, engine.ReasonInstanceMissing, "Delete",
				"%s %q is gone, so nothing was removed from it", o.instanceKind, o.instanceName)
			return nil
		}
		return ctrlcommon.CleanupApp(ctx, c, recorder, obj, instance.connection, pvrclient.Definition(o.app.APIVersion), o.app.Name)
	}
	if done, after := ctrlcommon.HandleLifecycle(ctx, c, recorder, obj, ctrlcommon.Lifecycle{Remove: cleanup}); done {
		return ctrl.Result{RequeueAfter: after}, nil
	}
	if missing {
		ctrlcommon.UpdateStatusUnreachable(ctx, c, obj, engine.ReasonReferenceNotFound,
			fmt.Sprintf("%s %q not found", o.instanceKind, o.instanceName))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	claimant, err := olderClaim(ctx, c, o)
	if err != nil {
		return ctrl.Result{}, err
	}
	if claimant != "" {
		ctrlcommon.UpdateStatusUnreachable(ctx, c, obj, engine.ReasonNameConflict,
			fmt.Sprintf("%q already configures %q in %s %q", claimant, o.name, o.instanceKind, o.instanceName))
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	hc, wait := connectPVR(ctx, c, obj, instance.connection, o.app)
	if wait != nil {
		return *wait, nil
	}
	opts, err := o.options(ctx)
	if err != nil {
		reason, after := ctrlcommon.AppRefFailure(err)
		ctrlcommon.UpdateStatusUnreachable(ctx, c, obj, reason, err.Error())
		return ctrl.Result{RequeueAfter: after}, nil
	}

	interval := ctrlcommon.ReconcileInterval(instance.policy)
	policy := ctrlcommon.Policy(instance.policy)
	// The object owns only the item it names, so prune replaces it on a rename.
	policy.Prune = true
	result, err := pvrclient.Reconcile(ctx, hc, o.app.APIVersion, nil, withApp(opts, o.app), policy, obj.GetManagedResources())
	if err != nil {
		ctrlcommon.UpdateStatus(ctx, c, obj, false, engine.ReasonSyncFailed, err.Error())
		return ctrl.Result{RequeueAfter: interval}, nil
	}
	if id, ok := idOf(ctx, hc, "/api/"+o.app.APIVersion+o.path, o.name); ok {
		o.setID(id)
	}
	ctrlcommon.UpdateStatusManaged(obj, result.Managed)
	ctrlcommon.EmitPruneEvents(recorder, obj, result.Pruned)
	ctrlcommon.ReportResult(ctx, c, obj, policy.Observe, result)
	return ctrl.Result{RequeueAfter: interval}, nil
}

// olderClaim names an older resource of the same kind that configures the same
// object in the same instance. That one keeps the object, so two resources
// never take turns rewriting it or delete it from under each other.
func olderClaim(ctx context.Context, c client.Client, o pvrObject) (string, error) {
	list := o.list.DeepCopyObject().(client.ObjectList)
	if err := c.List(ctx, list, client.InNamespace(o.obj.GetNamespace())); err != nil {
		return "", err
	}
	items, err := apimeta.ExtractList(list)
	if err != nil {
		return "", err
	}
	var claimant client.Object
	for _, item := range items {
		other, ok := item.(client.Object)
		if !ok || other.GetName() == o.obj.GetName() || !older(other, o.obj) {
			continue
		}
		if instance, name := o.claim(other); instance != o.instanceName || name != o.name {
			continue
		}
		if claimant == nil || older(other, claimant) {
			claimant = other
		}
	}
	if claimant == nil {
		return "", nil
	}
	return claimant.GetName(), nil
}

func older(a, b client.Object) bool {
	ta, tb := a.GetCreationTimestamp(), b.GetCreationTimestamp()
	if !ta.Equal(&tb) {
		return ta.Before(&tb)
	}
	return a.GetName() < b.GetName()
}

// idOf finds the ID the app gave the object with this name.
func idOf(ctx context.Context, hc *engine.HTTPClient, path, name string) (int32, bool) {
	items, err := hc.GetJSONList(ctx, path)
	if err != nil {
		return 0, false
	}
	for _, item := range items {
		if item["name"] == name {
			if id, ok := item["id"].(float64); ok {
				return int32(id), true
			}
		}
	}
	return 0, false
}

// objectsOf requeues the objects that name a changed instance.
func objectsOf(c client.Client, list client.ObjectList) handler.MapFunc {
	return func(ctx context.Context, instance client.Object) []reconcile.Request {
		objects := list.DeepCopyObject().(client.ObjectList)
		if err := c.List(ctx, objects, client.InNamespace(instance.GetNamespace()),
			client.MatchingFields{instanceRefIndex: instance.GetName()}); err != nil {
			return nil
		}
		items, err := apimeta.ExtractList(objects)
		if err != nil {
			return nil
		}
		reqs := make([]reconcile.Request, 0, len(items))
		for _, item := range items {
			if o, ok := item.(client.Object); ok {
				reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(o)})
			}
		}
		return reqs
	}
}

// downloadClientAppRefs returns the reference an <App>DownloadClient holds.
func downloadClientAppRefs(o client.Object) []commonv1alpha1.AppRef {
	var ref commonv1alpha1.AppRef
	switch o := o.(type) {
	case *pvrv1alpha1.SonarrDownloadClient:
		ref = o.Spec.AppRef
	case *pvrv1alpha1.RadarrDownloadClient:
		ref = o.Spec.AppRef
	case *pvrv1alpha1.LidarrDownloadClient:
		ref = o.Spec.AppRef
	case *pvrv1alpha1.ReadarrDownloadClient:
		ref = o.Spec.AppRef
	}
	if ref == (commonv1alpha1.AppRef{}) {
		return nil
	}
	return []commonv1alpha1.AppRef{ref}
}

// instanceChanged passes an instance's spec changes and changes to whether it
// synced. An object can name tags and profiles the instance creates, so it is
// retried once the instance syncs rather than at its next interval.
func instanceChanged() predicate.Predicate {
	return predicate.Or(predicate.GenerationChangedPredicate{}, predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return syncedStatus(e.ObjectOld) != syncedStatus(e.ObjectNew)
		},
		CreateFunc:  func(event.CreateEvent) bool { return false },
		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	})
}

func syncedStatus(obj client.Object) metav1.ConditionStatus {
	cr, ok := obj.(ctrlcommon.ConfigResource)
	if !ok {
		return ""
	}
	if cond := apimeta.FindStatusCondition(*cr.GetConditions(), engine.ConditionSynced); cond != nil {
		return cond.Status
	}
	return ""
}
