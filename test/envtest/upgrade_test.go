package envtest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// TestUpgradeFromPreviousRelease stores the previous release's samples under
// its CRDs, upgrades the CRDs to the current ones, and checks that the stored
// resources still read back and that applying the current samples over them
// succeeds. Server-side apply fails on stored objects whose managed fields no
// longer fit the schema, such as an object field that became a list.
func TestUpgradeFromPreviousRelease(t *testing.T) {
	previous := os.Getenv("PREVIOUS_RELEASE")
	if previous == "" || os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("PREVIOUS_RELEASE or KUBEBUILDER_ASSETS unset; run via `make test-upgrade`")
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join(previous, "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	require.NoError(t, err, "starting control plane with the previous CRDs")
	t.Cleanup(func() { _ = env.Stop() })
	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	ctx := context.Background()

	stored := applySamplesFrom(ctx, t, c, filepath.Join(previous, "config", "samples"))
	require.NotEmpty(t, stored, "the previous release has no samples to store")

	paths, err := filepath.Glob(filepath.Join("..", "..", "config", "crd", "bases", "*.yaml"))
	require.NoError(t, err)
	for _, path := range paths {
		for _, crd := range decode(t, path) {
			require.NoError(t, c.Apply(ctx, client.ApplyConfigurationFromUnstructured(crd),
				client.FieldOwner("helm"), client.ForceOwnership), "upgrading %s", crd.GetName())
			waitEstablished(ctx, t, c, crd.GetName())
		}
	}

	for _, obj := range stored {
		got := &unstructured.Unstructured{}
		got.SetGroupVersionKind(obj.GroupVersionKind())
		assert.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(obj), got), "reading back %s", describe(obj))
	}

	applySamplesFrom(ctx, t, c, filepath.Join("..", "..", "config", "samples"))

	var rejected []string
	for _, obj := range stored {
		if msg := rejectedByCurrentSchema(ctx, t, c, obj); msg != "" {
			rejected = append(rejected, describe(obj)+": "+msg)
		}
	}
	if len(rejected) > 0 {
		t.Logf("previous samples the current CRDs reject, which need upgrade notes:\n%s", strings.Join(rejected, "\n"))
	}
}

// applySamplesFrom server-side applies every resource in a samples directory,
// each file in its own namespace, as a user re-applying their manifests would.
// A CRD that has only just changed can serve the old schema for a moment, so a
// rejection is retried before it counts.
func applySamplesFrom(ctx context.Context, t *testing.T, c client.Client, dir string) []*unstructured.Unstructured {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	require.NoError(t, err)
	var applied []*unstructured.Unstructured
	for _, path := range paths {
		ns := namespaceFor(path)
		if err := c.Create(ctx, &corev1.Namespace{Name: ns}); err != nil {
			require.True(t, apierrors.IsAlreadyExists(err), "creating namespace %s: %v", ns, err)
		}
		for _, obj := range decode(t, path) {
			if !strings.HasPrefix(obj.GetAPIVersion(), "arrchitect.dev/") {
				continue
			}
			obj.SetNamespace(ns)
			var err error
			for range 5 {
				apply := client.ApplyConfigurationFromUnstructured(obj.DeepCopy())
				if err = c.Apply(ctx, apply, client.FieldOwner("user")); err == nil {
					break
				}
				time.Sleep(time.Second)
			}
			if assert.NoError(t, err, "%s: applying %s", filepath.Base(path), describe(obj)) {
				applied = append(applied, obj)
			}
		}
	}
	return applied
}

func waitEstablished(ctx context.Context, t *testing.T, c client.Client, name string) {
	t.Helper()
	require.Eventually(t, func() bool {
		crd := &unstructured.Unstructured{}
		crd.SetAPIVersion("apiextensions.k8s.io/v1")
		crd.SetKind("CustomResourceDefinition")
		if c.Get(ctx, client.ObjectKey{Name: name}, crd) != nil {
			return false
		}
		conditions, _, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
		for _, condition := range conditions {
			if m, ok := condition.(map[string]any); ok && m["type"] == "Established" && m["status"] == "True" {
				return true
			}
		}
		return false
	}, 30*time.Second, 200*time.Millisecond, "%s is not established", name)
}

// rejectedByCurrentSchema creates a copy of a stored object in a scratch
// namespace as a dry run, so it meets every current rule rather than the
// ratcheting an update gets, and returns the API server's reason if refused.
func rejectedByCurrentSchema(
	ctx context.Context, t *testing.T, c client.Client, obj *unstructured.Unstructured,
) string {
	t.Helper()
	const scratch = "upgrade-dry-run"
	if err := c.Create(ctx, &corev1.Namespace{Name: scratch}); err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err, "creating namespace %s", scratch)
	}
	probe := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": obj.GetAPIVersion(), "kind": obj.GetKind(),
		"metadata": map[string]any{"name": obj.GetName(), "namespace": scratch},
		"spec":     obj.Object["spec"],
	}}
	if err := c.Create(ctx, probe, client.DryRunAll); err != nil {
		return err.Error()
	}
	return ""
}

func describe(obj *unstructured.Unstructured) string {
	return fmt.Sprintf("%s %s/%s", obj.GetKind(), obj.GetNamespace(), obj.GetName())
}
