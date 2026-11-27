package appref

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/controller/secretref"
)

// index indexes resources by the app resources they refer to, as key values.
const index = ".spec.appRef"

func key(kind, name string) string {
	return kind + "/" + name
}

func keys(refs []commonv1alpha1.AppRef) []string {
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, key(ref.Kind, ref.Name))
	}
	return out
}

// Watch indexes obj's kind by the references refsOf returns, and requeues the
// resources that refer to an app resource of one of kinds when it changes, or
// when a Secret it uses does.
func Watch(
	mgr ctrl.Manager, b *builder.Builder, apps Apps, obj client.Object, list client.ObjectList,
	refsOf func(client.Object) []commonv1alpha1.AppRef, kinds ...string,
) (*builder.Builder, error) {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), obj, index, func(o client.Object) []string {
		return keys(refsOf(o))
	})
	if err != nil {
		return nil, err
	}
	c := mgr.GetClient()
	watched := watchable(apps, mgr.GetScheme(), mgr.GetRESTMapper(), kinds)
	for _, k := range watched {
		target := appKinds[k].newObj()
		if err := secretref.Index(mgr, target); err != nil {
			return nil, err
		}
		b = b.Watches(target, handler.EnqueueRequestsFromMapFunc(referrersOf(c, list, k)),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	}
	if len(watched) > 0 {
		b = b.WatchesMetadata(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(secretUsers(c, list, watched)))
	}
	return b, nil
}

// watchable keeps the kinds whose app the operator runs, the API server serves
// and the scheme knows. The cache can't read any other kind, and a read would
// wait forever.
func watchable(apps Apps, scheme *runtime.Scheme, mapper meta.RESTMapper, kinds []string) []string {
	var out []string
	for _, k := range kinds {
		if !apps.Runs(k) {
			continue
		}
		if _, _, err := scheme.ObjectKinds(appKinds[k].newObj()); err != nil {
			continue
		}
		if _, err := mapper.RESTMapping(schema.GroupKind{Group: "arrchitect.dev", Kind: k}); err != nil {
			continue
		}
		out = append(out, k)
	}
	return out
}

// referrersOf requeues the resources that refer to a changed app resource of
// kind.
func referrersOf(c client.Reader, list client.ObjectList, kind string) handler.MapFunc {
	return func(ctx context.Context, target client.Object) []reconcile.Request {
		return referrers(ctx, c, list, kind, target)
	}
}

// secretUsers requeues the resources that refer to an app resource of one of
// kinds that uses a changed Secret, which Watch indexed by Secret name.
func secretUsers(c client.Reader, list client.ObjectList, kinds []string) handler.MapFunc {
	return func(ctx context.Context, secret client.Object) []reconcile.Request {
		var reqs []reconcile.Request
		for _, k := range kinds {
			targets := appKinds[k].newList()
			if err := c.List(ctx, targets, client.InNamespace(secret.GetNamespace()), secretref.Using(secret.GetName())); err != nil {
				log.FromContext(ctx).Error(err, "listing app resources that use a changed Secret")
				continue
			}
			items, err := meta.ExtractList(targets)
			if err != nil {
				log.FromContext(ctx).Error(err, "reading app resources that use a changed Secret")
				continue
			}
			for _, item := range items {
				if target, ok := item.(client.Object); ok {
					reqs = append(reqs, referrers(ctx, c, list, k, target)...)
				}
			}
		}
		return reqs
	}
}

func referrers(ctx context.Context, c client.Reader, list client.ObjectList, kind string, target client.Object) []reconcile.Request {
	objs := list.DeepCopyObject().(client.ObjectList)
	if err := c.List(ctx, objs, client.InNamespace(target.GetNamespace()),
		client.MatchingFields{index: key(kind, target.GetName())}); err != nil {
		log.FromContext(ctx).Error(err, "listing resources that refer to a changed app resource")
		return nil
	}
	items, err := meta.ExtractList(objs)
	if err != nil {
		log.FromContext(ctx).Error(err, "reading resources that refer to a changed app resource")
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
