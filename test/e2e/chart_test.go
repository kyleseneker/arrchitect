//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
	"sigs.k8s.io/e2e-framework/third_party/helm"
	"sigs.k8s.io/yaml"

	commonv1alpha1 "github.com/kyleseneker/arrchitect/api/common/v1alpha1"
	pvrv1alpha1 "github.com/kyleseneker/arrchitect/api/pvr/v1alpha1"
)

const (
	chartRef     = "oci://ghcr.io/kyleseneker/charts/arrchitect"
	release      = "arrchitect"
	operatorNS   = "arrchitect"
	appNS        = "media"
	localChart   = "../../chart/arrchitect"
	syncDeadline = 5 * time.Minute
)

// TestChart installs the previous release's chart when there is one, upgrades
// to the local chart and image, and checks a SonarrConfig reconciles against a
// real Sonarr throughout. PREVIOUS_RELEASE names the release, such as v0.1.0.
func TestChart(t *testing.T) {
	apiKey := randomHex(t)
	f := features.New("install, upgrade and uninstall").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			startSonarr(ctx, t, cfg, apiKey)
			return ctx
		}).
		Assess("the previous release reconciles", installPreviousRelease).
		Assess("the local chart installs or upgrades and reconciles", installLocalChart(apiKey)).
		Assess("the cluster serves the chart's CRDs", servesChartCRDs).
		Assess("uninstalling keeps the CRDs and resources", uninstallKeepsCRDs).
		Feature()
	testenv.Test(t, f)
}

func installPreviousRelease(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	previous := os.Getenv("PREVIOUS_RELEASE")
	if previous == "" {
		t.Skip("no earlier release to upgrade from")
	}
	require.NoError(t, helm.New(cfg.KubeconfigFile()).RunInstall(
		helm.WithName(release), helm.WithChart(chartRef), helm.WithVersion(strings.TrimPrefix(previous, "v")),
		helm.WithNamespace(operatorNS), helm.WithArgs("--create-namespace"), helm.WithWait(), helm.WithTimeout("5m")))
	applySonarrConfig(ctx, t, cfg)
	waitSynced(t, cfg, time.Time{})
	return ctx
}

func installLocalChart(apiKey string) features.Func {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		repository, tag, _ := strings.Cut(image, ":")
		upgradedAt := time.Now()
		require.NoError(t, helm.New(cfg.KubeconfigFile()).RunUpgrade(
			helm.WithName(release), helm.WithChart(localChart), helm.WithNamespace(operatorNS),
			helm.WithArgs("--install", "--create-namespace", "--set", "image.repository="+repository,
				"--set", "image.tag="+tag, "--set", "image.pullPolicy=Never"),
			helm.WithWait(), helm.WithTimeout("5m")))
		applySonarrConfig(ctx, t, cfg)
		waitSynced(t, cfg, upgradedAt)

		var stdout, stderr bytes.Buffer
		require.NoError(t, cfg.Client().Resources().ExecInDeployment(ctx, appNS, "sonarr",
			[]string{"curl", "-fsS", "-H", "X-Api-Key: " + apiKey, "http://localhost:8989/api/v3/rootfolder"},
			&stdout, &stderr), stderr.String())
		assert.Regexp(t, regexp.MustCompile(`"path":\s*"/tv/?"`), stdout.String(), "Sonarr has no /tv root folder")
		return ctx
	}
}

func servesChartCRDs(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	paths, err := filepath.Glob(filepath.Join(localChart, "files", "crds", "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var want apiextensionsv1.CustomResourceDefinition
		require.NoError(t, yaml.Unmarshal(data, &want), path)
		var live apiextensionsv1.CustomResourceDefinition
		require.NoError(t, cfg.Client().Resources().Get(ctx, want.Name, "", &live), want.Name)
		assert.Equal(t, schemas(want), schemas(live), "%s differs from the chart", want.Name)
	}
	return ctx
}

func uninstallKeepsCRDs(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	require.NoError(t, helm.New(cfg.KubeconfigFile()).RunUninstall(
		helm.WithName(release), helm.WithNamespace(operatorNS), helm.WithWait()))
	var crd apiextensionsv1.CustomResourceDefinition
	assert.NoError(t, cfg.Client().Resources().Get(ctx, "sonarrconfigs.arrchitect.dev", "", &crd))
	var config pvrv1alpha1.SonarrConfig
	assert.NoError(t, cfg.Client().Resources().Get(ctx, "sonarr", appNS, &config))
	return ctx
}

func startSonarr(ctx context.Context, t *testing.T, cfg *envconf.Config, apiKey string) {
	t.Helper()
	r := cfg.Client().Resources()
	labels := map[string]string{"app": "sonarr"}
	emptyDir := corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
	for _, obj := range []k8s.Object{
		&corev1.Namespace{Name: appNS},
		&corev1.Secret{
			Name: "sonarr", Namespace: appNS,
			StringData: map[string]string{"api-key": apiKey},
		},
		&appsv1.Deployment{
			Name: "sonarr", Namespace: appNS,
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "sonarr",
							Image: sonarrImage(t),
							Env: []corev1.EnvVar{
								{Name: "PUID", Value: "1000"},
								{Name: "PGID", Value: "1000"},
								{Name: "SONARR__AUTH__APIKEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
									Name: "sonarr", Key: "api-key",
								}}},
							},
							Ports: []corev1.ContainerPort{{ContainerPort: 8989}},
							ReadinessProbe: &corev1.Probe{
								HTTPGet: &corev1.HTTPGetAction{Path: "/ping", Port: intstr.FromInt32(8989)}},
							VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: "/config"}, {Name: "tv", MountPath: "/tv"}},
						}},
						Volumes: []corev1.Volume{{Name: "config", VolumeSource: emptyDir}, {Name: "tv", VolumeSource: emptyDir}},
					},
				},
			},
		},
		&corev1.Service{
			Name: "sonarr", Namespace: appNS,
			Spec: corev1.ServiceSpec{Selector: labels, Ports: []corev1.ServicePort{{Port: 8989}}},
		},
	} {
		require.NoError(t, r.Create(ctx, obj), "creating %T", obj)
	}
	require.NoError(t, wait.For(conditions.New(r).DeploymentAvailable("sonarr", appNS), wait.WithTimeout(5*time.Minute)),
		"Sonarr did not become available")
}

// applySonarrConfig creates the resource, or leaves it as it is once created.
func applySonarrConfig(ctx context.Context, t *testing.T, cfg *envconf.Config) {
	t.Helper()
	config := &pvrv1alpha1.SonarrConfig{
		Name: "sonarr", Namespace: appNS,
		Spec: pvrv1alpha1.SonarrConfigSpec{
			Connection: commonv1alpha1.AppConnection{
				URL:             "http://sonarr." + appNS + ".svc.cluster.local:8989",
				APIKeySecretRef: commonv1alpha1.SecretKeyRef{Name: "sonarr", Key: "api-key"},
			},
			RootFolders: []commonv1alpha1.RootFolder{{Path: "/tv"}},
		},
	}
	var existing pvrv1alpha1.SonarrConfig
	if cfg.Client().Resources().Get(ctx, config.Name, config.Namespace, &existing) == nil {
		return
	}
	require.NoError(t, cfg.Client().Resources().Create(ctx, config))
}

// waitSynced waits until the operator has reconciled the resource after since,
// with Ready and Synced both true.
func waitSynced(t *testing.T, cfg *envconf.Config, since time.Time) {
	t.Helper()
	config := &pvrv1alpha1.SonarrConfig{Name: "sonarr", Namespace: appNS}
	err := wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(config, func(obj k8s.Object) bool {
		c := obj.(*pvrv1alpha1.SonarrConfig)
		return c.Status.LastSyncTime != nil && c.Status.LastSyncTime.After(since) &&
			apimeta.IsStatusConditionTrue(c.Status.Conditions, "Ready") &&
			apimeta.IsStatusConditionTrue(c.Status.Conditions, "Synced")
	}), wait.WithTimeout(syncDeadline), wait.WithInterval(5*time.Second))
	if err != nil {
		var got pvrv1alpha1.SonarrConfig
		_ = cfg.Client().Resources().Get(context.Background(), "sonarr", appNS, &got)
		require.NoError(t, err, "SonarrConfig never reconciled after %s; status: %+v", since, got.Status)
	}
}

// sonarrImage reads the version the compat suite pins, so both suites test the
// same Sonarr.
func sonarrImage(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../compat/images_test.go")
	require.NoError(t, err)
	m := regexp.MustCompile(`(?m)^\s*sonarrImage\s*=\s*"([^"]+)"`).FindSubmatch(data)
	require.NotNil(t, m, "no sonarrImage pin in the compat suite")
	return string(m[1])
}

func schemas(crd apiextensionsv1.CustomResourceDefinition) map[string]*apiextensionsv1.CustomResourceValidation {
	out := map[string]*apiextensionsv1.CustomResourceValidation{}
	for _, v := range crd.Spec.Versions {
		out[v.Name] = v.Schema
	}
	return out
}

func randomHex(t *testing.T) string {
	t.Helper()
	b := make([]byte, 16)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}
