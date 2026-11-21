//go:build compat

// Package compat reconciles every integration against a real, pinned application
// container. Each case proves three things for that application version: the
// operator reaches Synced, the application reflects the declared settings, and a
// second pass makes no writes, so nothing the operator sends is rewritten by the
// application into a value the operator then sees as drift. A case with an
// update also proves the application takes a change to an item it already has.
//
// Run with `make test-compat`. Containers run through testcontainers-go, on docker
// or, with CONTAINER_TOOL=podman, on the podman machine.
package compat

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	automationv1alpha1 "github.com/kyleseneker/arrchitect/api/automation/v1alpha1"
	curationv1alpha1 "github.com/kyleseneker/arrchitect/api/curation/v1alpha1"
	downloadsv1alpha1 "github.com/kyleseneker/arrchitect/api/downloads/v1alpha1"
	indexersv1alpha1 "github.com/kyleseneker/arrchitect/api/indexers/v1alpha1"
	mediaserversv1alpha1 "github.com/kyleseneker/arrchitect/api/mediaservers/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
	requestsv1alpha1 "github.com/kyleseneker/arrchitect/api/requests/v1alpha1"
	subtitlesv1alpha1 "github.com/kyleseneker/arrchitect/api/subtitles/v1alpha1"
	transcodev1alpha1 "github.com/kyleseneker/arrchitect/api/transcode/v1alpha1"
	ctrlcommon "github.com/kyleseneker/arrchitect/internal/controller/common"
	"github.com/kyleseneker/arrchitect/internal/engine"
	"github.com/kyleseneker/arrchitect/internal/export"
)

var (
	// hostIP is the address the operator and the containers use to reach each
	// other and the suite's fake services.
	hostIP string
	k8s    client.Client
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	if os.Getenv("KUBEBUILDER_ASSETS") == "" {
		fmt.Fprintln(os.Stderr, "KUBEBUILDER_ASSETS unset; run via `make test-compat`")
		return 1
	}
	var err error
	if hostIP, err = outboundIP(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "starting control plane: %v\n", err)
		return 1
	}
	defer func() { _ = env.Stop() }()

	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme, automationv1alpha1.AddToScheme, curationv1alpha1.AddToScheme,
		downloadsv1alpha1.AddToScheme, indexersv1alpha1.AddToScheme, mediaserversv1alpha1.AddToScheme,
		pvrv1alpha1.AddToScheme, requestsv1alpha1.AddToScheme, subtitlesv1alpha1.AddToScheme,
		transcodev1alpha1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			fmt.Fprintf(os.Stderr, "registering scheme: %v\n", err)
			return 1
		}
	}
	if k8s, err = client.New(cfg, client.Options{Scheme: scheme}); err != nil {
		fmt.Fprintf(os.Stderr, "building client: %v\n", err)
		return 1
	}
	return m.Run()
}

// outboundIP is the source address of the default route. ARRCHITECT_COMPAT_HOST
// overrides it when that address is not reachable from containers.
func outboundIP() (string, error) {
	if ip := os.Getenv("ARRCHITECT_COMPAT_HOST"); ip != "" {
		return ip, nil
	}
	conn, err := net.Dial("udp", "192.0.2.1:9")
	if err != nil {
		return "", fmt.Errorf("finding the host's outbound address: %w", err)
	}
	defer func() { _ = conn.Close() }()
	return conn.LocalAddr().(*net.UDPAddr).IP.String(), nil
}

// compatCase is one application version under test.
type compatCase struct {
	// fixture is a file under testdata/; ${VAR} references are filled from
	// vars. Its first document is the case's own kind, and any others are
	// objects of a kind in objects, or app resources the others refer to.
	fixture string
	vars    map[string]string
	// secrets become one Secret named "compat-credentials" in the test namespace.
	secrets    map[string]string
	newObj     func() ctrlcommon.ConfigResource
	reconciler func(client.Client) reconcile.Reconciler
	objects    map[string]compatKind
	proxy      *recordingProxy
	// read classifies requests the application treats as reads; nil means GET only.
	read isRead
	// observe is true when the integration supports driftPolicy: observe.
	observe bool
	// verify reads the application directly after the first reconcile.
	verify func(t *testing.T)
	// update is a JSON patch (RFC 6902) to the case's own resource that changes
	// items the first reconcile created, so the operator updates them in place.
	// verifyUpdate reads the application after that reconcile.
	update       string
	verifyUpdate func(t *testing.T)
	// newerFields are JSON pointers into the fixture's first document to
	// fields an older supported release lacks; runs at older pins leave them
	// out.
	newerFields []string
	// export names the app for arrchitect export, whose output must then
	// describe the application exactly: observe mode finds nothing different.
	export string
	// exportAPIKey supplies the API key export reads with, where the fixture
	// signs in another way.
	exportAPIKey func(t *testing.T) string
	// cleanup, for an integration that supports deletionPolicy: delete, checks
	// the application after the resources are deleted with that policy. owned
	// is what they recorded creating, read just before the delete.
	cleanup func(t *testing.T, owned map[string][]string)
}

// compatKind is a kind of object a fixture can hold besides the case's own.
type compatKind struct {
	newObj     func() ctrlcommon.ConfigResource
	reconciler func(client.Client) reconcile.Reconciler
}

// resource is one fixture document as created in the test namespace.
type resource struct {
	key        types.NamespacedName
	newObj     func() ctrlcommon.ConfigResource
	reconciler reconcile.Reconciler
}

// create creates obj, decoded from a fixture or export, along with what
// reconciles it.
func (tc compatCase) create(t *testing.T, obj ctrlcommon.ConfigResource) resource {
	t.Helper()
	kind := tc.kindOf(t, obj)
	require.NoError(t, k8s.Create(context.Background(), obj),
		"creating %s %s", reflect.TypeOf(obj).Elem().Name(), obj.GetName())
	return resource{key: client.ObjectKeyFromObject(obj), newObj: kind.newObj, reconciler: kind.reconciler(k8s)}
}

func (tc compatCase) kindOf(t *testing.T, obj client.Object) compatKind {
	t.Helper()
	name := reflect.TypeOf(obj).Elem().Name()
	if name == reflect.TypeOf(tc.newObj()).Elem().Name() {
		return compatKind{newObj: tc.newObj, reconciler: tc.reconciler}
	}
	kind, ok := tc.objects[name]
	require.True(t, ok, "the case has no kind %s", name)
	return kind
}

// decode reads a document of the case's own kind or one of its objects' kinds.
func (tc compatCase) decode(t *testing.T, doc string) ctrlcommon.ConfigResource {
	t.Helper()
	var meta metav1.TypeMeta
	require.NoError(t, yaml.Unmarshal([]byte(doc), &meta))
	obj := tc.newObj()
	if kind, ok := tc.objects[meta.Kind]; ok {
		obj = kind.newObj()
	}
	require.NoError(t, yaml.UnmarshalStrict([]byte(doc), obj), "decoding a %s", meta.Kind)
	return obj
}

// reference decodes a document of a kind the case neither is nor reconciles:
// an app resource for the case's resources to refer to. It is created but
// never reconciled, since the test has already set that app up.
func (tc compatCase) reference(t *testing.T, doc string) (client.Object, bool) {
	t.Helper()
	var meta metav1.TypeMeta
	require.NoError(t, yaml.Unmarshal([]byte(doc), &meta))
	if _, ok := tc.objects[meta.Kind]; ok || meta.Kind == reflect.TypeOf(tc.newObj()).Elem().Name() {
		return nil, false
	}
	o, err := k8s.Scheme().New(schema.FromAPIVersionAndKind(meta.APIVersion, meta.Kind))
	require.NoError(t, err, "a %s document", meta.Kind)
	obj, ok := o.(client.Object)
	require.True(t, ok)
	require.NoError(t, yaml.UnmarshalStrict([]byte(doc), obj), "decoding a %s", meta.Kind)
	return obj, true
}

func checkCompat(t *testing.T, tc compatCase) {
	t.Helper()
	ctx := context.Background()
	read := tc.read
	if read == nil {
		read = getOnly
	}

	ns := &corev1.Namespace{GenerateName: "compat-"}
	require.NoError(t, k8s.Create(ctx, ns), "creating namespace")
	secret := &corev1.Secret{Name: "compat-credentials", Namespace: ns.Name, StringData: tc.secrets}
	require.NoError(t, k8s.Create(ctx, secret), "creating secret")

	docs := loadFixture(t, tc.fixture, tc.vars)
	if !atNewest() {
		docs[0] = withoutFields(t, docs[0], tc.newerFields)
	}
	fixture := make([]client.Object, 0, len(docs))
	resources := make([]resource, 0, len(docs))
	for _, doc := range docs {
		if ref, ok := tc.reference(t, doc); ok {
			ref.SetNamespace(ns.Name)
			require.NoError(t, k8s.Create(ctx, ref),
				"creating %s %s", ref.GetObjectKind().GroupVersionKind().Kind, ref.GetName())
			continue
		}
		obj := tc.decode(t, doc)
		obj.SetNamespace(ns.Name)
		fixture = append(fixture, obj.DeepCopyObject().(client.Object))
		resources = append(resources, tc.create(t, obj))
	}
	instance := resources[0]

	if !reconcileAll(t, resources, "first reconcile") {
		t.Logf("writes during the first reconcile:\n%s", formatRequests(tc.proxy.writes(read)))
		t.FailNow()
	}
	if tc.verify != nil {
		tc.verify(t)
	}

	tc.proxy.reset()
	if !reconcileAll(t, resources, "second reconcile") {
		t.Logf("requests during the second reconcile:\n%s", formatRequests(tc.proxy.all()))
		t.FailNow()
	}
	writes := tc.proxy.writes(read)
	assert.Empty(t, writes, "second reconcile wrote to the application; these settings do not read back as written:\n%s",
		formatRequests(writes))

	if tc.update != "" {
		checkUpdate(t, tc, resources, read)
	}

	if !tc.observe {
		return
	}
	patch := client.RawPatch(types.MergePatchType, []byte(`{"spec":{"reconcile":{"driftPolicy":"observe"}}}`))
	require.NoError(t, k8s.Patch(ctx, instance.newObjAt(t), patch), "switching to observe")
	tc.proxy.reset()
	require.True(t, reconcileAll(t, resources, "observe reconcile"))
	writes = tc.proxy.writes(read)
	assert.Empty(t, writes, "observe mode wrote to the application:\n%s", formatRequests(writes))

	if tc.export != "" {
		checkExport(t, tc, fixture, read)
	}
	if tc.cleanup != nil {
		checkCleanup(t, tc, resources)
	}
}

// checkUpdate applies the case's update and reconciles: the application takes
// the change in place, and the pass after it writes nothing.
func checkUpdate(t *testing.T, tc compatCase, resources []resource, read isRead) {
	t.Helper()
	patch := client.RawPatch(types.JSONPatchType, []byte(tc.update))
	require.NoError(t, k8s.Patch(context.Background(), resources[0].newObjAt(t), patch), "updating")
	tc.proxy.reset()
	if !reconcileAll(t, resources, "update reconcile") {
		t.Logf("requests during the update reconcile:\n%s", formatRequests(tc.proxy.all()))
		t.FailNow()
	}
	assert.NotEmpty(t, tc.proxy.writes(read), "the update reconcile wrote nothing")
	if tc.verifyUpdate != nil {
		tc.verifyUpdate(t)
	}
	tc.proxy.reset()
	require.True(t, reconcileAll(t, resources, "reconcile after update"))
	writes := tc.proxy.writes(read)
	assert.Empty(t, writes, "the reconcile after the update wrote again:\n%s", formatRequests(writes))
}

// newObjAt reads the resource as stored.
func (r resource) newObjAt(t *testing.T) ctrlcommon.ConfigResource {
	t.Helper()
	obj := r.newObj()
	require.NoError(t, k8s.Get(context.Background(), r.key, obj), "reading %s", r.key)
	return obj
}

// reconcileAll reconciles each resource in turn until its status describes its
// current generation, and reports whether all of them synced.
func reconcileAll(t *testing.T, resources []resource, stage string) bool {
	t.Helper()
	ok := true
	for _, r := range resources {
		obj := reconcileUntilCurrent(t, r.reconciler, r.key, r.newObj)
		ok = assertSynced(t, obj, stage+": "+r.key.Name) && ok
	}
	return ok
}

// checkCleanup deletes the resources with deletionPolicy: delete, objects
// before their instance, then lets the case check that what the operator
// created is gone from the application.
func checkCleanup(t *testing.T, tc compatCase, resources []resource) {
	t.Helper()
	ctx := context.Background()
	instance := resources[0]
	patch := client.RawPatch(types.MergePatchType,
		[]byte(`{"spec":{"reconcile":{"driftPolicy":"enforce","deletionPolicy":"delete"}}}`))
	require.NoError(t, k8s.Patch(ctx, instance.newObjAt(t), patch), "setting deletionPolicy: delete")
	require.True(t, reconcileAll(t, resources, "reconcile with deletionPolicy: delete"))
	owned := map[string][]string{}
	for _, r := range resources {
		for kind, names := range r.newObjAt(t).GetManagedResources() {
			owned[kind] = append(owned[kind], names...)
		}
	}
	require.NotEmpty(t, owned, "the resources recorded nothing they created, so cleanup proves nothing")
	for _, r := range append(resources[1:], instance) {
		deleteAndRelease(t, r)
	}
	tc.cleanup(t, owned)
}

func deleteAndRelease(t *testing.T, r resource) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, k8s.Delete(ctx, r.newObjAt(t)), "deleting %s", r.key)
	for range 5 {
		_, err := r.reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: r.key})
		require.NoError(t, err, "reconcile during deletion")
		if apierrors.IsNotFound(k8s.Get(ctx, r.key, r.newObj())) {
			return
		}
	}
	require.FailNow(t, "the resource was not released after cleanup", "%s", r.key)
}

func checkExport(t *testing.T, tc compatCase, fixture []client.Object, read isRead) {
	t.Helper()
	ctx := context.Background()
	namespace := fixture[0].GetNamespace()
	apiKey := tc.secrets["api-key"]
	if tc.exportAPIKey != nil {
		apiKey = tc.exportAPIKey(t)
	}
	res, err := export.Run(ctx, export.Options{
		App: tc.export, URL: tc.proxy.url(), APIKey: apiKey, Name: "exported", Namespace: namespace,
		Username: tc.secrets["username"], Password: tc.secrets["password"], Token: tc.secrets["token"],
	})
	require.NoError(t, err, "exporting %s", tc.export)
	out, err := res.YAML()
	require.NoError(t, err)
	for _, w := range res.Warnings {
		t.Logf("export warning: %s", w)
	}
	exportedSpec, fixtureSpec := specOf(t, res.Config), specOf(t, fixture[0])
	for section, value := range fixtureSpec {
		if section == "connection" || section == "reconcile" {
			continue
		}
		if !assert.Contains(t, exportedSpec, section, "export left out %s, which the fixture configures:\n%s", section, out) {
			continue
		}
		fields, ok := value.(map[string]any)
		exported, _ := exportedSpec[section].(map[string]any)
		for field := range fields {
			// Export can't tell which resource describes an app, so it writes
			// the address a reference stands for.
			if field == "appRef" {
				continue
			}
			if ok && !slices.Contains(res.Unreadable, section+"."+field) {
				assert.Contains(t, exported, field, "export left out %s.%s, which the fixture configures:\n%s", section, field, out)
			}
		}
	}
	exportedNames := map[string][]string{}
	for _, obj := range res.Objects {
		kind := reflect.TypeOf(obj).Elem().Name()
		exportedNames[kind] = append(exportedNames[kind], fmt.Sprint(specOf(t, obj)["name"]))
	}
	for _, obj := range fixture[1:] {
		kind := reflect.TypeOf(obj).Elem().Name()
		assert.Contains(t, exportedNames[kind], specOf(t, obj)["name"],
			"export left out the %s the fixture configures:\n%s", kind, out)
	}

	for _, key := range res.Unfilled {
		res.Secret.StringData[key] = "unreadable"
	}
	if res.Secret != nil {
		require.NoError(t, k8s.Create(ctx, res.Secret), "creating the exported Secret")
	}
	obj, ok := res.Config.(ctrlcommon.ConfigResource)
	require.True(t, ok)
	resources := make([]resource, 0, 1+len(res.Objects))
	resources = append(resources, tc.create(t, obj))
	patch := client.RawPatch(types.MergePatchType, []byte(`{"spec":{"reconcile":{"driftPolicy":"observe"}}}`))
	require.NoError(t, k8s.Patch(ctx, obj, patch), "switching the exported config to observe")
	for _, o := range res.Objects {
		cr, ok := o.(ctrlcommon.ConfigResource)
		require.True(t, ok)
		resources = append(resources, tc.create(t, cr))
	}

	tc.proxy.reset()
	if !reconcileAll(t, resources, "exported config") {
		t.Logf("exported config:\n%s\nobserved differences: %v", out, observedDrift(t, tc.export))
		t.FailNow()
	}
	writes := tc.proxy.writes(read)
	assert.Empty(t, writes, "reconciling the exported config wrote to the application:\n%s", formatRequests(writes))
}

// reconcileUntilCurrent reconciles until status describes the current generation;
// the first pass may only add the finalizer.
func reconcileUntilCurrent(
	t *testing.T, r reconcile.Reconciler, key types.NamespacedName, newObj func() ctrlcommon.ConfigResource,
) ctrlcommon.ConfigResource {
	t.Helper()
	// Debug verbosity includes each failed request's full response, which status
	// conditions omit because an application may echo credentials back.
	ctx := logf.IntoContext(context.Background(), testr.NewWithOptions(t, testr.Options{Verbosity: 1}))
	for range 5 {
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: key})
		require.NoError(t, err, "reconcile")
		obj := newObj()
		require.NoError(t, k8s.Get(ctx, key, obj), "reading %s", key)
		if observed := obj.GetObservedGeneration(); observed != nil && *observed == obj.GetGeneration() {
			return obj
		}
	}
	require.FailNow(t, "status never reached the current generation", "%s", key)
	return nil
}

func assertSynced(t *testing.T, obj ctrlcommon.ConfigResource, stage string) bool {
	t.Helper()
	for _, c := range *obj.GetConditions() {
		if c.Type == engine.ConditionSynced {
			return assert.Equal(t, metav1.ConditionTrue, c.Status,
				"%s: Synced=%s (%s): %s", stage, c.Status, c.Reason, c.Message)
		}
	}
	return assert.Fail(t, "no Synced condition", stage)
}

// loadFixture returns the documents of a testdata file with its variables
// filled in.
func loadFixture(t *testing.T, name string, vars map[string]string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	missing := map[string]bool{}
	expanded := os.Expand(string(raw), func(key string) string {
		value, ok := vars[key]
		if !ok {
			missing[key] = true
		}
		return value
	})
	require.Empty(t, missing, "%s references unset variables", name)
	var docs []string
	for doc := range strings.SplitSeq(expanded, "\n---\n") {
		if strings.TrimSpace(doc) != "" {
			docs = append(docs, doc)
		}
	}
	return docs
}

// withoutFields removes the fields at JSON pointers from a YAML document.
func withoutFields(t *testing.T, doc string, pointers []string) string {
	t.Helper()
	if len(pointers) == 0 {
		return doc
	}
	var obj map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(doc), &obj))
	for _, pointer := range pointers {
		parts := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
		parent := obj
		for _, part := range parts[:len(parts)-1] {
			next, ok := parent[part].(map[string]any)
			require.True(t, ok, "%s: no object at %s", pointer, part)
			parent = next
		}
		last := parts[len(parts)-1]
		require.Contains(t, parent, last, "%s isn't in the fixture", pointer)
		delete(parent, last)
	}
	out, err := yaml.Marshal(obj)
	require.NoError(t, err)
	return string(out)
}

func formatRequests(rs []request) string {
	lines := make([]string, len(rs))
	for i, r := range rs {
		lines[i] = "  " + r.String()
	}
	return strings.Join(lines, "\n")
}

// waitFor polls until ready returns nil, for applications that start slowly.
func waitFor(t *testing.T, what string, timeout time.Duration, ready func() error) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var err error
	for time.Now().Before(deadline) {
		if err = ready(); err == nil {
			return
		}
		time.Sleep(time.Second)
	}
	require.NoError(t, err, "%s not ready after %s", what, timeout)
}

func observedDrift(t *testing.T, app string) []string {
	t.Helper()
	families, err := ctrlmetrics.Registry.Gather()
	require.NoError(t, err)
	var drift []string
	for _, family := range families {
		if family.GetName() != "arrchitect_drift_observed_total" {
			continue
		}
		for _, m := range family.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["app"] == app {
				drift = append(drift, labels["resource_type"]+"/"+labels["name"])
			}
		}
	}
	return drift
}

func specOf(t *testing.T, obj client.Object) map[string]any {
	t.Helper()
	raw, err := json.Marshal(obj)
	require.NoError(t, err)
	var m struct {
		Spec map[string]any `json:"spec"`
	}
	require.NoError(t, json.Unmarshal(raw, &m))
	return m.Spec
}

// ownedGone checks that none of the items a resource recorded creating is left
// in an application reconciled through def, except kept ones (as kind/name),
// which the app refuses to delete while something it keeps still uses them.
func ownedGone(hc *engine.HTTPClient, def engine.AppDefinition, kept ...string) func(*testing.T, map[string][]string) {
	return func(t *testing.T, owned map[string][]string) {
		for _, ep := range def.Resources {
			if !ep.Prunable || len(owned[ep.Name]) == 0 {
				continue
			}
			items, err := hc.GetJSONList(context.Background(), ep.Path)
			require.NoError(t, err)
			left := map[string]bool{}
			for _, item := range items {
				key := fmt.Sprint(item[ep.MatchField])
				if ep.MatchKey != nil {
					key = ep.MatchKey(item)
				}
				left[key] = true
			}
			for _, name := range owned[ep.Name] {
				if slices.Contains(kept, ep.Name+"/"+name) {
					assert.True(t, left[name], "%s %q should have been kept", ep.Name, name)
					continue
				}
				assert.False(t, left[name], "%s %q, which the operator created, is still there", ep.Name, name)
			}
		}
	}
}
