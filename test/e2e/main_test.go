//go:build e2e

// Package e2e installs the chart into a throwaway kind cluster and reconciles a
// real Sonarr. kind writes the clusters it creates and deletes into the file
// KUBECONFIG names, so TestMain points it at a temporary file before anything
// else runs: the tests never read or write your default kubeconfig.
package e2e

import (
	"log"
	"os"
	"path/filepath"
	"testing"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"

	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
)

const (
	clusterName = "arrchitect-e2e"
	image       = "arrchitect:e2e"
)

var testenv env.Environment

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "arrchitect-e2e-")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.Setenv("KUBECONFIG", filepath.Join(dir, "kubeconfig")); err != nil {
		log.Fatal(err)
	}
	for _, add := range []func(*runtime.Scheme) error{pvrv1alpha1.AddToScheme, apiextensionsv1.AddToScheme} {
		if err := add(scheme.Scheme); err != nil {
			log.Fatal(err)
		}
	}

	kindPath := os.Getenv("KIND")
	if kindPath == "" {
		kindPath = "kind"
	}
	testenv = env.New()
	testenv.Setup(
		envfuncs.CreateClusterWithOpts(kind.NewProvider().WithPath(kindPath), clusterName),
		envfuncs.LoadImageToCluster(clusterName, image),
	)
	testenv.Finish(envfuncs.DestroyCluster(clusterName))
	code := testenv.Run(m)
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
