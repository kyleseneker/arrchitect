// Package appref follows an appRef to the app resource it names, and requeues
// the resources that refer to one when it, or a Secret it uses, changes.
package appref

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	"github.com/kyleseneker/arrchitect/internal/failure"
)

// App is what an AppRef resolves to: where the app is, how to reach it over
// TLS, and the Secrets that hold its credentials.
type App struct {
	Kind              string
	URL               string
	TLS               *commonv1alpha1.TLSConfig
	APIKeySecretRef   *commonv1alpha1.SecretKeyRef
	UsernameSecretRef *commonv1alpha1.SecretKeyRef
	PasswordSecretRef *commonv1alpha1.SecretKeyRef
	TokenSecretRef    *commonv1alpha1.SecretKeyRef
}

type appKind struct {
	newObj  func() client.Object
	newList func() client.ObjectList
	app     func(client.Object) App
}

func withAPIKey(conn commonv1alpha1.AppConnection) App {
	return App{URL: conn.URL, TLS: conn.TLS, APIKeySecretRef: &conn.APIKeySecretRef}
}

var appKinds = map[string]appKind{
	"Sonarr": {func() client.Object { return &pvrv1alpha1.Sonarr{} },
		func() client.ObjectList { return &pvrv1alpha1.SonarrList{} },
		func(o client.Object) App { return withAPIKey(o.(*pvrv1alpha1.Sonarr).Spec.Connection) }},
	"Radarr": {func() client.Object { return &pvrv1alpha1.Radarr{} },
		func() client.ObjectList { return &pvrv1alpha1.RadarrList{} },
		func(o client.Object) App { return withAPIKey(o.(*pvrv1alpha1.Radarr).Spec.Connection) }},
	"Lidarr": {func() client.Object { return &pvrv1alpha1.Lidarr{} },
		func() client.ObjectList { return &pvrv1alpha1.LidarrList{} },
		func(o client.Object) App { return withAPIKey(o.(*pvrv1alpha1.Lidarr).Spec.Connection) }},
	"Readarr": {func() client.Object { return &pvrv1alpha1.Readarr{} },
		func() client.ObjectList { return &pvrv1alpha1.ReadarrList{} },
		func(o client.Object) App { return withAPIKey(o.(*pvrv1alpha1.Readarr).Spec.Connection) }},
	"Prowlarr": {func() client.Object { return &indexersv1alpha1.Prowlarr{} },
		func() client.ObjectList { return &indexersv1alpha1.ProwlarrList{} },
		func(o client.Object) App { return withAPIKey(o.(*indexersv1alpha1.Prowlarr).Spec.Connection) }},
	"Sabnzbd": {func() client.Object { return &downloadsv1alpha1.Sabnzbd{} },
		func() client.ObjectList { return &downloadsv1alpha1.SabnzbdList{} },
		func(o client.Object) App { return withAPIKey(o.(*downloadsv1alpha1.Sabnzbd).Spec.Connection) }},
	"Seerr": {func() client.Object { return &requestsv1alpha1.Seerr{} },
		func() client.ObjectList { return &requestsv1alpha1.SeerrList{} },
		func(o client.Object) App {
			conn := o.(*requestsv1alpha1.Seerr).Spec.Connection
			return App{URL: conn.URL, TLS: conn.TLS, APIKeySecretRef: conn.APIKeySecretRef}
		}},
	"Jellyfin": {func() client.Object { return &mediaserversv1alpha1.Jellyfin{} },
		func() client.ObjectList { return &mediaserversv1alpha1.JellyfinList{} },
		func(o client.Object) App {
			spec := o.(*mediaserversv1alpha1.Jellyfin).Spec
			return App{URL: spec.Connection.URL, TLS: spec.Connection.TLS,
				UsernameSecretRef: &spec.AdminUser.UsernameSecretRef, PasswordSecretRef: &spec.AdminUser.PasswordSecretRef,
				APIKeySecretRef: spec.Connection.APIKeySecretRef}
		}},
	"Plex": {func() client.Object { return &mediaserversv1alpha1.Plex{} },
		func() client.ObjectList { return &mediaserversv1alpha1.PlexList{} },
		func(o client.Object) App {
			conn := o.(*mediaserversv1alpha1.Plex).Spec.Connection
			return App{URL: conn.URL, TLS: conn.TLS, TokenSecretRef: &conn.TokenSecretRef}
		}},
	"QBittorrent": {func() client.Object { return &downloadsv1alpha1.QBittorrent{} },
		func() client.ObjectList { return &downloadsv1alpha1.QBittorrentList{} },
		func(o client.Object) App {
			conn := o.(*downloadsv1alpha1.QBittorrent).Spec.Connection
			return App{URL: conn.URL, TLS: conn.TLS, UsernameSecretRef: &conn.UsernameSecretRef, PasswordSecretRef: &conn.PasswordSecretRef}
		}},
}

// Apps are the apps the operator runs, by controller name. It reads and
// watches only those apps' kinds, the ones its RBAC covers. Nil is every app.
type Apps map[string]bool

// Runs reports whether the operator runs kind's app.
func (a Apps) Runs(kind string) bool {
	return a == nil || a[strings.ToLower(kind)]
}

// ErrNotFound is wrapped in Resolve's error for a resource that doesn't exist.
var ErrNotFound = errors.New("not found")

// Invalid is an appRef that can't be followed until a spec changes: a kind the
// connection doesn't take, an app the operator doesn't run, or a resource
// without what the connection needs from it.
func Invalid(format string, args ...any) error {
	return failure.InvalidConfig(fmt.Errorf(format, args...))
}

// Resolve reads the resource ref names in namespace. Its errors name the
// reference, never what the resource holds.
func Resolve(ctx context.Context, c client.Reader, apps Apps, namespace string, ref commonv1alpha1.AppRef) (App, error) {
	kind, ok := appKinds[ref.Kind]
	if !ok {
		return App{}, Invalid("%s %q: unsupported kind", ref.Kind, ref.Name)
	}
	if !apps.Runs(ref.Kind) {
		return App{}, Invalid("%s %q: the operator doesn't run the %s controller", ref.Kind, ref.Name, strings.ToLower(ref.Kind))
	}
	obj := kind.newObj()
	if err := c.Get(ctx, types.NamespacedName{Namespace: namespace, Name: ref.Name}, obj); err != nil {
		if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return App{}, fmt.Errorf("%s %q %w", ref.Kind, ref.Name, ErrNotFound)
		}
		return App{}, fmt.Errorf("reading %s %q: %w", ref.Kind, ref.Name, err)
	}
	app := kind.app(obj)
	app.Kind = ref.Kind
	return app, nil
}

// ResolveAddress is Resolve for a connection that takes the app's URL in
// parts.
func ResolveAddress(ctx context.Context, c client.Reader, apps Apps, namespace string, ref commonv1alpha1.AppRef) (App, Address, error) {
	app, err := Resolve(ctx, c, apps, namespace, ref)
	if err != nil {
		return App{}, Address{}, err
	}
	addr, err := splitURL(app.URL)
	if err != nil {
		return App{}, Address{}, Invalid("%s %q has no usable connection URL", ref.Kind, ref.Name)
	}
	return app, addr, nil
}

// Address is a URL in the parts some apps take it as.
type Address struct {
	Host string
	Port int32
	TLS  bool
	// Base is the URL's path, without a trailing slash; empty at the root.
	Base string
}

// splitURL breaks an app's URL into an Address, with the scheme's default port
// when the URL has none.
func splitURL(raw string) (Address, error) {
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
