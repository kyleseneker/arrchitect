package common

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/engine"
)

// App is what an AppRef resolves to: where the app is, and the Secrets that
// hold its credentials.
type App struct {
	Kind              string
	URL               string
	APIKeySecretRef   *commonv1alpha1.SecretKeyRef
	UsernameSecretRef *commonv1alpha1.SecretKeyRef
	PasswordSecretRef *commonv1alpha1.SecretKeyRef
	TokenSecretRef    *commonv1alpha1.SecretKeyRef
}

type appKind struct {
	newObj func() client.Object
	app    func(client.Object) App
}

func withAPIKey(conn commonv1alpha1.AppConnection) App {
	return App{URL: conn.URL, APIKeySecretRef: &conn.APIKeySecretRef}
}

var appKinds = map[string]appKind{
	"Sonarr": {func() client.Object { return &pvrv1alpha1.Sonarr{} },
		func(o client.Object) App { return withAPIKey(o.(*pvrv1alpha1.Sonarr).Spec.Connection) }},
	"Radarr": {func() client.Object { return &pvrv1alpha1.Radarr{} },
		func(o client.Object) App { return withAPIKey(o.(*pvrv1alpha1.Radarr).Spec.Connection) }},
	"Lidarr": {func() client.Object { return &pvrv1alpha1.Lidarr{} },
		func(o client.Object) App { return withAPIKey(o.(*pvrv1alpha1.Lidarr).Spec.Connection) }},
	"Readarr": {func() client.Object { return &pvrv1alpha1.Readarr{} },
		func(o client.Object) App { return withAPIKey(o.(*pvrv1alpha1.Readarr).Spec.Connection) }},
	"Prowlarr": {func() client.Object { return &indexersv1alpha1.Prowlarr{} },
		func(o client.Object) App { return withAPIKey(o.(*indexersv1alpha1.Prowlarr).Spec.Connection) }},
	"Sabnzbd": {func() client.Object { return &downloadsv1alpha1.Sabnzbd{} },
		func(o client.Object) App { return withAPIKey(o.(*downloadsv1alpha1.Sabnzbd).Spec.Connection) }},
	"Seerr": {func() client.Object { return &requestsv1alpha1.Seerr{} },
		func(o client.Object) App {
			conn := o.(*requestsv1alpha1.Seerr).Spec.Connection
			return App{URL: conn.URL, APIKeySecretRef: conn.APIKeySecretRef}
		}},
	"Jellyfin": {func() client.Object { return &mediaserversv1alpha1.Jellyfin{} },
		func(o client.Object) App {
			spec := o.(*mediaserversv1alpha1.Jellyfin).Spec
			return App{URL: spec.Connection.URL,
				UsernameSecretRef: &spec.AdminUser.UsernameSecretRef, PasswordSecretRef: &spec.AdminUser.PasswordSecretRef}
		}},
	"Plex": {func() client.Object { return &mediaserversv1alpha1.Plex{} },
		func(o client.Object) App {
			conn := o.(*mediaserversv1alpha1.Plex).Spec.Connection
			return App{URL: conn.URL, TokenSecretRef: &conn.TokenSecretRef}
		}},
	"QBittorrent": {func() client.Object { return &downloadsv1alpha1.QBittorrent{} },
		func(o client.Object) App {
			conn := o.(*downloadsv1alpha1.QBittorrent).Spec.Connection
			return App{URL: conn.URL, UsernameSecretRef: &conn.UsernameSecretRef, PasswordSecretRef: &conn.PasswordSecretRef}
		}},
}

var errAppRefNotFound = errors.New("not found")

// IsAppRefNotFound reports whether err is ResolveAppRef's for a resource that
// doesn't exist.
func IsAppRefNotFound(err error) bool {
	return errors.Is(err, errAppRefNotFound)
}

// AppRefReason is the status reason for a ResolveAppRef error.
func AppRefReason(err error) string {
	if IsAppRefNotFound(err) {
		return engine.ReasonReferenceNotFound
	}
	return engine.ReasonSyncFailed
}

// ResolveAppRef reads the resource ref names in namespace. Its errors name the
// reference, never what the resource holds.
func ResolveAppRef(ctx context.Context, c client.Reader, namespace string, ref commonv1alpha1.AppRef) (App, error) {
	kind, ok := appKinds[ref.Kind]
	if !ok {
		return App{}, fmt.Errorf("%s %q: unsupported kind", ref.Kind, ref.Name)
	}
	obj := kind.newObj()
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, obj); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return App{}, fmt.Errorf("%s %q %w", ref.Kind, ref.Name, errAppRefNotFound)
		}
		return App{}, fmt.Errorf("reading %s %q: %w", ref.Kind, ref.Name, err)
	}
	app := kind.app(obj)
	app.Kind = ref.Kind
	return app, nil
}

// ResolveAppAddress is ResolveAppRef for a connection that takes the app's URL
// in parts.
func ResolveAppAddress(ctx context.Context, c client.Reader, namespace string, ref commonv1alpha1.AppRef) (App, Address, error) {
	app, err := ResolveAppRef(ctx, c, namespace, ref)
	if err != nil {
		return App{}, Address{}, err
	}
	addr, err := SplitURL(app.URL)
	if err != nil {
		return App{}, Address{}, fmt.Errorf("%s %q has no usable connection URL", ref.Kind, ref.Name)
	}
	return app, addr, nil
}

// ResolveDownloadClient returns dc with its appRef, if any, replaced by the
// referenced download client's kind, address and credentials.
func ResolveDownloadClient(
	ctx context.Context, c client.Reader, namespace string, dc commonv1alpha1.DownloadClient,
) (commonv1alpha1.DownloadClient, error) {
	if dc.AppRef == (commonv1alpha1.AppRef{}) {
		return dc, nil
	}
	app, addr, err := ResolveAppAddress(ctx, c, namespace, dc.AppRef)
	if err != nil {
		return dc, fmt.Errorf("download client %q appRef: %w", dc.Name, err)
	}
	dc.Implementation, dc.Host, dc.Port, dc.UseSsl, dc.UrlBase = app.Kind, addr.Host, addr.Port, &addr.TLS, addr.Base
	switch app.Kind {
	case "QBittorrent":
		dc.Protocol = "torrent"
		dc.UsernameSecretRef, dc.PasswordSecretRef = *app.UsernameSecretRef, *app.PasswordSecretRef
	case "Sabnzbd":
		dc.Protocol = "usenet"
		dc.APIKeySecretRef = *app.APIKeySecretRef
	default:
		return dc, fmt.Errorf("download client %q appRef: %s isn't a download client", dc.Name, app.Kind)
	}
	return dc, nil
}

// Address is a URL in the parts some apps take it as.
type Address struct {
	Host string
	Port int32
	TLS  bool
	// Base is the URL's path, without a trailing slash; empty at the root.
	Base string
}

// SplitURL breaks an app's URL into an Address, with the scheme's default port
// when the URL has none.
func SplitURL(raw string) (Address, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return Address{}, fmt.Errorf("not an app URL")
	}
	addr := Address{Host: u.Hostname(), TLS: u.Scheme == "https", Base: strings.TrimSuffix(u.Path, "/")}
	switch p := u.Port(); {
	case p != "":
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil || n == 0 {
			return Address{}, fmt.Errorf("not an app URL")
		}
		addr.Port = int32(n)
	case addr.TLS:
		addr.Port = 443
	default:
		addr.Port = 80
	}
	return addr, nil
}

// appRefIndex indexes resources by the app resources they refer to, as
// appRefKey values.
const appRefIndex = ".spec.appRef"

func appRefKey(kind, name string) string {
	return kind + "/" + name
}

func appRefKeys(refs []commonv1alpha1.AppRef) []string {
	keys := make([]string, 0, len(refs))
	for _, ref := range refs {
		keys = append(keys, appRefKey(ref.Kind, ref.Name))
	}
	return keys
}

// WatchAppRefs indexes obj's kind by the references refsOf returns and requeues
// the ones that refer to a changed app resource, for each of kinds the API
// server serves and the manager's scheme knows. A kind whose CRD the chart left
// out is skipped.
func WatchAppRefs(
	mgr ctrl.Manager, b *builder.Builder, obj client.Object, list client.ObjectList,
	refsOf func(client.Object) []commonv1alpha1.AppRef, kinds ...string,
) (*builder.Builder, error) {
	err := mgr.GetFieldIndexer().IndexField(context.Background(), obj, appRefIndex, func(o client.Object) []string {
		return appRefKeys(refsOf(o))
	})
	if err != nil {
		return nil, err
	}
	for _, k := range kinds {
		target := appKinds[k].newObj()
		if _, _, err := mgr.GetScheme().ObjectKinds(target); err != nil {
			continue
		}
		if _, err := mgr.GetRESTMapper().RESTMapping(schema.GroupKind{Group: "arrchitect.dev", Kind: k}); err != nil {
			continue
		}
		b = b.Watches(target, handler.EnqueueRequestsFromMapFunc(referrersOf(mgr.GetClient(), list, k)),
			builder.WithPredicates(predicate.GenerationChangedPredicate{}))
	}
	return b, nil
}

func referrersOf(c client.Client, list client.ObjectList, kind string) handler.MapFunc {
	return func(ctx context.Context, target client.Object) []reconcile.Request {
		objs := list.DeepCopyObject().(client.ObjectList)
		if err := c.List(ctx, objs, client.InNamespace(target.GetNamespace()),
			client.MatchingFields{appRefIndex: appRefKey(kind, target.GetName())}); err != nil {
			log.FromContext(ctx).Error(err, "listing resources that refer to a changed app resource")
			return nil
		}
		return requestsFor(objs, func(client.Object) bool { return true })
	}
}

// EnqueueForSecretThroughRefs is EnqueueForSecret for resources that can also
// use a Secret through an AppRef.
func EnqueueForSecretThroughRefs(c client.Client, list client.ObjectList, refsOf func(client.Object) []commonv1alpha1.AppRef) handler.EventHandler {
	return handler.EnqueueRequestsFromMapFunc(secretReferrers(c, list, refsOf))
}

func secretReferrers(c client.Client, list client.ObjectList, refsOf func(client.Object) []commonv1alpha1.AppRef) handler.MapFunc {
	return func(ctx context.Context, secret client.Object) []reconcile.Request {
		return listMatching(ctx, c, list, secret.GetNamespace(), func(o client.Object) bool {
			if ReferencesSecret(o, secret.GetName()) {
				return true
			}
			for _, ref := range refsOf(o) {
				kind, ok := appKinds[ref.Kind]
				if !ok {
					continue
				}
				target := kind.newObj()
				if c.Get(ctx, types.NamespacedName{Namespace: o.GetNamespace(), Name: ref.Name}, target) == nil &&
					ReferencesSecret(target, secret.GetName()) {
					return true
				}
			}
			return false
		})
	}
}

func listMatching(ctx context.Context, c client.Client, list client.ObjectList, namespace string, match func(client.Object) bool) []reconcile.Request {
	objs := list.DeepCopyObject().(client.ObjectList)
	if err := c.List(ctx, objs, client.InNamespace(namespace)); err != nil {
		log.FromContext(ctx).Error(err, "listing resources to requeue")
		return nil
	}
	return requestsFor(objs, match)
}

func requestsFor(objs client.ObjectList, match func(client.Object) bool) []reconcile.Request {
	items, err := meta.ExtractList(objs)
	if err != nil {
		return nil
	}
	var reqs []reconcile.Request
	for _, item := range items {
		if o, ok := item.(client.Object); ok && match(o) {
			reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(o)})
		}
	}
	return reqs
}
