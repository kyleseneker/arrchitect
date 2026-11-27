// Package export reads an app's configuration and writes it as a config
// resource the operator can manage, with a Secret for the credentials.
package export

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	curationv1alpha1 "github.com/kyleseneker/arrchitect/api/curation/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	transcodev1alpha1 "github.com/kyleseneker/arrchitect/api/transcode/v1alpha1"
	autobrrclient "github.com/kyleseneker/arrchitect/internal/client/autobrr"
	bazarrclient "github.com/kyleseneker/arrchitect/internal/client/bazarr"
	qbclient "github.com/kyleseneker/arrchitect/internal/client/qbittorrent"
	tdarrclient "github.com/kyleseneker/arrchitect/internal/client/tdarr"
	"github.com/kyleseneker/arrchitect/internal/controller/pvr"
	"github.com/kyleseneker/arrchitect/internal/controller/secretref"
	"github.com/kyleseneker/arrchitect/internal/httpapi"
	"github.com/kyleseneker/arrchitect/internal/secrets"
)

const apiKeyKey = "api-key"

// Options selects the app to read and names what export writes.
type Options struct {
	App string
	// URL is where export reads the app; ConnectionURL, if set, is the URL the
	// operator will use instead, such as a Service name when URL is a port-forward.
	URL                string
	ConnectionURL      string
	APIKey             string
	Username           string
	Password           string
	Token              string
	Name               string
	Namespace          string
	SecretName         string
	Exclude            []string
	InsecureSkipVerify bool
	// Enforce leaves observe mode out, so the operator writes to the app as
	// soon as the config is applied.
	Enforce bool
}

// Result is an exported config and the Secret it reads credentials from.
// Unfilled lists the Secret keys whose values the app would not reveal, which
// the Secret leaves out; Warnings what the app has that the config cannot
// express; and Unreadable the spec fields, as section.field, that the app does
// not report.
type Result struct {
	Config client.Object
	// Objects are the app's objects that have kinds of their own, each naming
	// Config as its instance.
	Objects    []client.Object
	Secret     *corev1.Secret
	Unfilled   []string
	Warnings   []string
	Unreadable []string
}

type app struct {
	auth       httpapi.AuthType
	apiKey     bool
	login      bool
	keyInstead bool
	// hostPort is an app that refuses a request whose Host header names
	// another port than its own, as through a port-forward.
	hostPort bool
	token    bool
	options  []httpapi.HTTPClientOption
	newObj   func() client.Object
	export   func(r *reader) (map[string]any, error)
	// objects maps the spec sections whose items are resources of their own to
	// the kind each item becomes.
	objects map[string]objectKind
}

// objectKind is a kind an app's objects are exported as. ref is the spec
// field that names the instance.
type objectKind struct {
	kind   string
	ref    string
	newObj func() client.Object
}

var apps = map[string]app{
	"sonarr": withObjects(pvrApp(pvr.Sonarr, func() client.Object { return &pvrv1alpha1.Sonarr{} }),
		map[string]objectKind{
			"indexers": {kind: "SonarrIndexer", ref: "sonarrRef",
				newObj: func() client.Object { return &pvrv1alpha1.SonarrIndexer{} }},
			"downloadClients": {kind: "SonarrDownloadClient", ref: "sonarrRef",
				newObj: func() client.Object { return &pvrv1alpha1.SonarrDownloadClient{} }},
		}),
	"radarr": withObjects(pvrApp(pvr.Radarr, func() client.Object { return &pvrv1alpha1.Radarr{} }),
		map[string]objectKind{
			"indexers": {kind: "RadarrIndexer", ref: "radarrRef",
				newObj: func() client.Object { return &pvrv1alpha1.RadarrIndexer{} }},
			"downloadClients": {kind: "RadarrDownloadClient", ref: "radarrRef",
				newObj: func() client.Object { return &pvrv1alpha1.RadarrDownloadClient{} }},
		}),
	"lidarr": withObjects(pvrApp(pvr.Lidarr, func() client.Object { return &pvrv1alpha1.Lidarr{} }),
		map[string]objectKind{
			"indexers": {kind: "LidarrIndexer", ref: "lidarrRef",
				newObj: func() client.Object { return &pvrv1alpha1.LidarrIndexer{} }},
			"downloadClients": {kind: "LidarrDownloadClient", ref: "lidarrRef",
				newObj: func() client.Object { return &pvrv1alpha1.LidarrDownloadClient{} }},
		}),
	"readarr": withObjects(pvrApp(pvr.Readarr, func() client.Object { return &pvrv1alpha1.Readarr{} }),
		map[string]objectKind{
			"indexers": {kind: "ReadarrIndexer", ref: "readarrRef",
				newObj: func() client.Object { return &pvrv1alpha1.ReadarrIndexer{} }},
			"downloadClients": {kind: "ReadarrDownloadClient", ref: "readarrRef",
				newObj: func() client.Object { return &pvrv1alpha1.ReadarrDownloadClient{} }},
		}),
	"prowlarr": {
		auth: httpapi.AuthAPIKey, apiKey: true, export: exportProwlarr,
		newObj: func() client.Object { return &indexersv1alpha1.Prowlarr{} },
	},
	"flaresolverr": {
		auth: httpapi.AuthNone, export: exportFlareSolverr,
		newObj: func() client.Object { return &indexersv1alpha1.FlareSolverr{} },
	},
	"qbittorrent": {
		auth: httpapi.AuthCookie, login: true, hostPort: true, export: exportQBittorrent,
		options: qbclient.HTTPOptions(),
		newObj:  func() client.Object { return &downloadsv1alpha1.QBittorrent{} },
	},
	"sabnzbd": {
		auth: httpapi.AuthNone, apiKey: true, export: exportSabnzbd,
		newObj: func() client.Object { return &downloadsv1alpha1.Sabnzbd{} },
	},
	"jellyfin": {
		auth: httpapi.AuthMediaBrowser, login: true, keyInstead: true, export: exportJellyfin,
		newObj: func() client.Object { return &mediaserversv1alpha1.Jellyfin{} },
	},
	"plex": {
		auth: httpapi.AuthPlexToken, token: true, export: exportPlex,
		newObj: func() client.Object { return &mediaserversv1alpha1.Plex{} },
	},
	"seerr": {
		auth: httpapi.AuthSession, apiKey: true, export: exportSeerr,
		newObj: func() client.Object { return &requestsv1alpha1.Seerr{} },
	},
	"tdarr": {
		auth: httpapi.AuthAPIKey, apiKey: true, export: exportTdarr,
		options: tdarrclient.HTTPOptions(),
		newObj:  func() client.Object { return &transcodev1alpha1.Tdarr{} },
	},
	"autobrr": {
		auth: httpapi.AuthAPIKey, apiKey: true, export: exportAutobrr,
		options: autobrrclient.HTTPOptions(),
		newObj:  func() client.Object { return &automationv1alpha1.Autobrr{} },
	},
	"maintainerr": {
		auth: httpapi.AuthNone, export: exportMaintainerr,
		newObj: func() client.Object { return &curationv1alpha1.Maintainerr{} },
	},
	"bazarr": {
		auth: httpapi.AuthAPIKey, apiKey: true, export: exportBazarr,
		options: bazarrclient.HTTPOptions(),
		newObj:  func() client.Object { return &subtitlesv1alpha1.Bazarr{} },
	},
}

// NeedsAPIKey reports whether export reads app with an API key.
func NeedsAPIKey(app string) bool {
	return apps[app].apiKey
}

// NeedsLogin reports whether export signs in to app with a username and password.
func NeedsLogin(app string) bool {
	return apps[app].login
}

// TakesAPIKey reports whether an API key can stand in for app's login.
func TakesAPIKey(app string) bool {
	return apps[app].keyInstead
}

// NeedsToken reports whether export reads app with a token.
func NeedsToken(app string) bool {
	return apps[app].token
}

// Apps lists the apps export supports.
func Apps() []string {
	return slices.Sorted(maps.Keys(apps))
}

// Run reads an app and returns a config that describes it.
func Run(ctx context.Context, o Options) (Result, error) {
	a, ok := apps[o.App]
	if !ok {
		return Result{}, fmt.Errorf("unsupported app %q; supported: %s", o.App, strings.Join(Apps(), ", "))
	}
	for _, secret := range []string{o.APIKey, o.Password, o.Token} {
		secrets.RememberSecret(secret)
	}
	if o.Name == "" {
		o.Name = o.App
	}
	if o.SecretName == "" {
		o.SecretName = o.Name + "-credentials"
	}
	if o.ConnectionURL == "" {
		o.ConnectionURL = o.URL
	}
	obj := a.newObj()
	sections := append(specFields(obj), slices.Sorted(maps.Keys(a.objects))...)
	for _, x := range o.Exclude {
		if !slices.Contains(sections, x) {
			return Result{}, fmt.Errorf("cannot exclude %q; %s has: %s", x, o.App, strings.Join(sections, ", "))
		}
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if o.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the user asked to skip verification
	}
	options := append([]httpapi.HTTPClientOption{
		httpapi.WithAPIKey(o.APIKey), httpapi.WithTransport(transport), httpapi.WithAppLabel(o.App),
	}, a.options...)
	if a.token {
		options = append(options, httpapi.WithPlexToken(o.Token))
	}
	hc, err := httpapi.NewHTTPClient(o.URL, a.auth, options...)
	if err != nil {
		return Result{}, err
	}

	r := &reader{
		ctx: ctx, hc: hc, apiKey: o.APIKey, username: o.Username, password: o.Password, token: o.Token,
		creds: &credentials{secret: o.SecretName, values: map[string]string{apiKeyKey: o.APIKey}},
	}
	spec, err := a.export(r)
	if err != nil {
		return Result{}, explain(o, err)
	}
	for _, x := range o.Exclude {
		delete(spec, x)
	}
	conn, _ := spec["connection"].(map[string]any)
	if conn == nil {
		conn = map[string]any{}
	}
	conn["url"] = o.ConnectionURL
	if a.apiKey || (a.keyInstead && o.APIKey != "") {
		conn["apiKeySecretRef"] = map[string]any{"name": o.SecretName, "key": apiKeyKey}
	}
	spec["connection"] = conn
	if !o.Enforce {
		spec["reconcile"] = map[string]any{"driftPolicy": "observe"}
	}
	if loopback(o.ConnectionURL) {
		r.warn("connection.url is %s, which the operator won't reach from inside the cluster; pass --connection-url",
			o.ConnectionURL)
	}
	objects, err := splitObjects(spec, a.objects, o)
	if err != nil {
		return Result{}, err
	}
	raw, err := json.Marshal(map[string]any{
		"apiVersion": "arrchitect.dev/v1alpha1",
		"kind":       reflect.TypeOf(obj).Elem().Name(),
		"metadata":   map[string]any{"name": o.Name, "namespace": o.Namespace},
		"spec":       spec,
	})
	if err != nil {
		return Result{}, err
	}
	if err := decodeStrict(raw, obj); err != nil {
		return Result{}, fmt.Errorf("reading %s into %T: %w", o.App, obj, err)
	}

	secret := &corev1.Secret{
		APIVersion: "v1", Kind: "Secret",
		Name: o.SecretName, Namespace: o.Namespace,
		StringData: map[string]string{},
	}
	var unfilled []string
	for _, key := range referencedKeys(append([]client.Object{obj}, objects...), o.SecretName) {
		if value := r.creds.values[key]; value != "" {
			secret.StringData[key] = value
		} else {
			unfilled = append(unfilled, key)
		}
	}
	if len(secret.StringData) == 0 && len(unfilled) == 0 {
		secret = nil
	}
	return Result{
		Config: obj, Objects: objects, Secret: secret, Unfilled: unfilled, Warnings: r.warnings, Unreadable: r.unreadable,
	}, nil
}

func withObjects(a app, objects map[string]objectKind) app {
	a.objects = objects
	return a
}

// splitObjects moves the items of each section that has a kind of its own out
// of spec, into resources that name the instance.
func splitObjects(spec map[string]any, kinds map[string]objectKind, o Options) ([]client.Object, error) {
	var objects []client.Object
	for _, section := range slices.Sorted(maps.Keys(kinds)) {
		k := kinds[section]
		items, _ := spec[section].([]any)
		delete(spec, section)
		for i, item := range items {
			fields, _ := item.(map[string]any)
			objSpec := maps.Clone(fields)
			objSpec[k.ref] = map[string]any{"name": o.Name}
			name, _ := fields["name"].(string)
			raw, err := json.Marshal(map[string]any{
				"apiVersion": "arrchitect.dev/v1alpha1",
				"kind":       k.kind,
				"metadata":   map[string]any{"name": objectName(o.Name, name, i), "namespace": o.Namespace},
				"spec":       objSpec,
			})
			if err != nil {
				return nil, err
			}
			obj := k.newObj()
			if err := decodeStrict(raw, obj); err != nil {
				return nil, fmt.Errorf("reading %s %q into %T: %w", section, name, obj, err)
			}
			objects = append(objects, obj)
		}
	}
	return objects, nil
}

var notNameChars = regexp.MustCompile(`[^a-z0-9]+`)

// objectName turns an object's name in the app, such as "NZBgeek (Prowlarr)",
// into a resource name under the instance's: "sonarr-nzbgeek-prowlarr".
func objectName(instance, name string, index int) string {
	slug := strings.Trim(notNameChars.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		slug = strconv.Itoa(index + 1)
	}
	out := instance + "-" + slug
	if len(out) > 253 {
		out = strings.TrimRight(out[:253], "-")
	}
	return out
}

// YAML renders the Secret and the config as one multi-document YAML stream.
func (r Result) YAML() ([]byte, error) {
	config, err := r.ConfigYAML()
	if err != nil || r.Secret == nil {
		return config, err
	}
	secret, err := r.SecretYAML()
	if err != nil {
		return nil, err
	}
	return append(append(secret, "---\n"...), config...), nil
}

// ConfigYAML renders the config and its objects, without the Secret.
func (r Result) ConfigYAML() ([]byte, error) {
	out, err := documentYAML(r.Config)
	if err != nil {
		return nil, err
	}
	for _, obj := range r.Objects {
		doc, err := documentYAML(obj)
		if err != nil {
			return nil, err
		}
		out = append(append(out, "---\n"...), doc...)
	}
	return out, nil
}

// SecretYAML renders the Secret alone, or nothing when the config reads no
// credentials.
func (r Result) SecretYAML() ([]byte, error) {
	if r.Secret == nil {
		return nil, nil
	}
	return documentYAML(r.Secret)
}

func documentYAML(obj any) ([]byte, error) {
	raw, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	delete(m, "status")
	if meta, ok := m["metadata"].(map[string]any); ok {
		delete(meta, "creationTimestamp")
	}
	return yaml.Marshal(m)
}

// explain says what a failed read most likely means for the URL and
// credentials given.
func explain(o Options, err error) error {
	var apiErr *httpapi.APIError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden):
		a := apps[o.App]
		flags := "--api-key"
		switch {
		case a.login && (o.APIKey == "" || !a.keyInstead):
			flags = "--username and --password"
		case a.token:
			flags = "--token"
		}
		if a.hostPort {
			return fmt.Errorf("reading %s at %s: the app refused the request (status %d); check %s, and that --url's "+
				"port is the app's own: it refuses a request addressed to any other, as through a port-forward to "+
				"another local port", o.App, o.URL, apiErr.StatusCode, flags)
		}
		return fmt.Errorf("reading %s at %s: the app refused the credentials (status %d); check %s",
			o.App, o.URL, apiErr.StatusCode, flags)
	case errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound:
		return fmt.Errorf("reading %s at %s: not found (status 404); check --url points at %s, including any base path",
			o.App, o.URL, o.App)
	case errors.As(err, &syntaxErr):
		return fmt.Errorf("reading %s at %s: the answer wasn't the app's API, such as a sign-in page from a proxy in front of it; "+
			"check --url", o.App, o.URL)
	}
	return fmt.Errorf("reading %s at %s: %w", o.App, o.URL, err)
}

// loopback reports whether rawURL names this machine, which a port-forward
// does and the operator's pod can't reach.
func loopback(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type credentials struct {
	secret string
	values map[string]string
}

var invalidKeyChars = regexp.MustCompile(`[^a-z0-9.]+`)

// configFields lists values and Secret references as fields, ordered by name.
func configFields[V any](values map[string]V, refs map[string]any) []any {
	fields := make([]any, 0, len(values)+len(refs))
	for _, name := range slices.Sorted(maps.Keys(values)) {
		fields = append(fields, map[string]any{"name": name, "value": values[name]})
	}
	for _, name := range slices.Sorted(maps.Keys(refs)) {
		fields = append(fields, map[string]any{"name": name, "valueFrom": map[string]any{"secretKeyRef": refs[name]}})
	}
	slices.SortFunc(fields, func(a, b any) int {
		return strings.Compare(a.(map[string]any)["name"].(string), b.(map[string]any)["name"].(string))
	})
	return fields
}

func (c *credentials) ref(value string, parts ...string) map[string]any {
	base := strings.Trim(invalidKeyChars.ReplaceAllString(strings.ToLower(strings.Join(parts, "-")), "-"), "-")
	key := base
	for i := 2; ; i++ {
		if _, taken := c.values[key]; !taken {
			break
		}
		key = fmt.Sprintf("%s-%d", base, i)
	}
	c.values[key] = value
	return map[string]any{"name": c.secret, "key": key}
}

func specFields(obj client.Object) []string {
	spec, _ := reflect.TypeOf(obj).Elem().FieldByName("Spec")
	var names []string
	for field := range spec.Type.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name != "" && name != "connection" && name != "reconcile" {
			names = append(names, name)
		}
	}
	return names
}

func referencedKeys(objs []client.Object, secret string) []string {
	var keys []string
	for _, ref := range secretref.Refs(objs) {
		if ref.Name == secret && !slices.Contains(keys, ref.Key) {
			keys = append(keys, ref.Key)
		}
	}
	return keys
}

type reader struct {
	ctx        context.Context
	hc         *httpapi.HTTPClient
	apiKey     string
	username   string
	password   string
	token      string
	prefix     string
	creds      *credentials
	tags       map[float64]string
	warnings   []string
	unreadable []string
}

type resource struct {
	key    string
	path   string
	schema string
	build  func(item map[string]any, defaults map[string]map[string]any) map[string]any
}

func (r *reader) warn(format string, args ...any) {
	r.warnings = append(r.warnings, fmt.Sprintf(format, args...))
}

func (r *reader) list(path string) ([]map[string]any, error) {
	return r.hc.GetJSONList(r.ctx, r.prefix+path)
}

func (r *reader) names(path, field string) (map[float64]string, error) {
	items, err := r.list(path)
	if err != nil {
		return nil, err
	}
	out := map[float64]string{}
	for _, item := range items {
		out[num(item["id"])], _ = item[field].(string)
	}
	return out, nil
}

func (r *reader) defaults(path string) (map[string]map[string]any, error) {
	schemas, err := r.list(path)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	for _, schema := range schemas {
		impl, _ := schema["implementation"].(string)
		values := map[string]any{}
		for _, f := range fieldList(schema) {
			name, _ := f["name"].(string)
			values[name] = f["value"]
		}
		out[impl] = values
	}
	return out, nil
}

func (r *reader) resources(spec map[string]any, resources []resource) error {
	for _, res := range resources {
		items, err := r.list(res.path)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			continue
		}
		var defaults map[string]map[string]any
		if res.schema != "" {
			if defaults, err = r.defaults(res.schema); err != nil {
				return err
			}
		}
		out := make([]any, 0, len(items))
		for _, item := range items {
			out = append(out, res.build(item, defaults))
		}
		spec[res.key] = out
	}
	return nil
}

// fields keeps provider fields that differ from the implementation's defaults,
// turning masked values into Secret references where the resource can read
// one (readsSecrets) and warning where it cannot.
func (r *reader) fields(item map[string]any, defaults map[string]map[string]any, skip map[string]bool, kind string, readsSecrets bool) []any {
	impl, _ := item["implementation"].(string)
	name, _ := item["name"].(string)
	var out []any
	for _, f := range fieldList(item) {
		key, _ := f["name"].(string)
		value := f["value"]
		switch {
		case skip[key]:
		case secrets.IsMasked(value) && readsSecrets:
			out = append(out, map[string]any{"name": key, "valueFrom": map[string]any{"secretKeyRef": r.creds.ref("", kind, name, key)}})
		case secrets.IsMasked(value):
			r.warn("%s %q: the app hides %s and the config can't read it from a Secret; set it in the app", kind, name, key)
		case reflect.DeepEqual(value, defaults[impl][key]):
		default:
			out = append(out, map[string]any{"name": key, "value": value})
		}
	}
	return out
}

func (r *reader) tagLabels(item map[string]any) []any {
	var labels []any
	ids, _ := item["tags"].([]any)
	for _, id := range ids {
		if label, ok := r.tags[num(id)]; ok {
			labels = append(labels, label)
		}
	}
	return labels
}

var notificationEvents = func() []string {
	var events []string
	t := reflect.TypeFor[commonv1alpha1.Notification]()
	for field := range t.Fields() {
		if field.Type == reflect.TypeFor[*bool]() {
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			events = append(events, name)
		}
	}
	return events
}()

func (r *reader) notification(item map[string]any, defaults map[string]map[string]any) map[string]any {
	n := pick(item, "name", "implementation")
	for _, event := range notificationEvents {
		v, ok := item[event]
		if !ok {
			continue
		}
		if supported, known := item["supports"+strings.ToUpper(event[:1])+event[1:]].(bool); known && !supported {
			continue
		}
		n[event] = v
	}
	contract(n, item)
	n["fields"] = r.fields(item, defaults, nil, "notification", true)
	n["tagLabels"] = r.tagLabels(item)
	return n
}

func tag(item map[string]any, _ map[string]map[string]any) map[string]any {
	return map[string]any{"label": item["label"]}
}

func protocol(out map[string]any) {
	if p, ok := out["protocol"].(string); ok {
		out["protocol"] = strings.ToLower(strings.TrimSuffix(p, "DownloadProtocol"))
	}
}

func contract(out, item map[string]any) {
	impl, _ := item["implementation"].(string)
	if contract, _ := item["configContract"].(string); contract != "" && contract != impl+"Settings" {
		out["configContract"] = contract
	}
}

func fieldValues(item map[string]any) map[string]any {
	values := map[string]any{}
	for _, f := range fieldList(item) {
		key, _ := f["name"].(string)
		values[key] = f["value"]
	}
	return values
}

func pick(item map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		if v, ok := item[k]; ok {
			out[k] = v
		}
	}
	return out
}

func list(v any) []map[string]any {
	raw, _ := v.([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func fieldList(item map[string]any) []map[string]any {
	return list(item["fields"])
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}

// decodeStrict decodes raw into obj and refuses a field obj doesn't have, so a
// misspelled key in a mapping fails rather than vanishing.
func decodeStrict(raw []byte, obj any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(obj)
}
