package secretref

import (
	"context"
	"reflect"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
)

// index indexes resources by the names of the Secrets they refer to.
const index = ".spec.secretRefs"

// Watch indexes obj's kind by the Secrets it refers to, and requeues the
// resources of that kind that refer to a changed Secret.
func Watch(mgr ctrl.Manager, b *builder.Builder, obj client.Object, list client.ObjectList) (*builder.Builder, error) {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), obj, index, names); err != nil {
		return nil, err
	}
	return b.WatchesMetadata(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(users(mgr.GetClient(), list))), nil
}

// users requeues the resources in list's kind that refer to a changed Secret.
func users(c client.Reader, list client.ObjectList) handler.MapFunc {
	return func(ctx context.Context, secret client.Object) []reconcile.Request {
		objs := list.DeepCopyObject().(client.ObjectList)
		if err := c.List(ctx, objs, client.InNamespace(secret.GetNamespace()),
			client.MatchingFields{index: secret.GetName()}); err != nil {
			log.FromContext(ctx).Error(err, "listing resources that use a changed Secret")
			return nil
		}
		items, err := meta.ExtractList(objs)
		if err != nil {
			log.FromContext(ctx).Error(err, "reading resources that use a changed Secret")
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

// names lists the Secrets obj refers to, each once.
func names(obj client.Object) []string {
	var out []string
	for _, ref := range Refs(obj) {
		if !slices.Contains(out, ref.Name) {
			out = append(out, ref.Name)
		}
	}
	return out
}

var secretKeyRefType = reflect.TypeFor[commonv1alpha1.SecretKeyRef]()

// Uses reports whether a config refers to the named Secret anywhere.
func Uses(config any, name string) bool {
	return slices.ContainsFunc(Refs(config), func(ref commonv1alpha1.SecretKeyRef) bool { return ref.Name == name })
}

// Refs lists every SecretKeyRef in a config. Every Secret reference in the API
// is a SecretKeyRef, so walking the config finds new ones as fields are added
// rather than relying on a list kept by hand.
func Refs(config any) []commonv1alpha1.SecretKeyRef {
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
