// Package operator runs the arrchitect manager: its flags, manager and
// controllers.
package operator

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strings"

	// Lets a kubeconfig sign in with the OIDC auth provider.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// Reconciler is a controller that registers itself with a manager.
type Reconciler interface {
	SetupWithManager(mgr ctrl.Manager) error
}

// Reconcilers sets several reconcilers up as one integration, for an app whose
// objects have kinds of their own.
type Reconcilers []Reconciler

func (rs Reconcilers) SetupWithManager(mgr ctrl.Manager) error {
	for _, r := range rs {
		if err := r.SetupWithManager(mgr); err != nil {
			return err
		}
	}
	return nil
}

// Deps are what a reconciler is built from: the manager's client and scheme,
// an event recorder named for the controller, and the apps the operator runs.
type Deps struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
	Apps     map[string]bool
}

// apiReaderClient is the manager's client with its uncached reader attached,
// for the status writes that must read an object's latest version.
type apiReaderClient struct {
	client.Client
	reader client.Reader
}

func (c apiReaderClient) APIReader() client.Reader { return c.reader }

// Controller is one integration the manager runs.
type Controller struct {
	Name string
	New  func(Deps) Reconciler
}

var setupLog = ctrl.Log.WithName("setup")

// Run parses the flags, starts the manager with the given schemes and
// controllers, and blocks until it stops. It exits the process on error.
func Run(addToSchemes []func(*runtime.Scheme) error, controllers ...Controller) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	for _, add := range addToSchemes {
		utilruntime.Must(add(scheme))
	}

	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var enableControllers string
	var watchNamespace string
	var disableControllers string
	var maxConcurrentReconciles int
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8443", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or set to 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	flag.StringVar(&enableControllers, "enable-controllers", "*",
		"Comma-separated list of controllers to enable (e.g., sonarr,radarr,prowlarr). Use * for all.")
	flag.StringVar(&disableControllers, "disable-controllers", "",
		"Comma-separated list of controllers to disable (applied after --enable-controllers).")
	flag.StringVar(&watchNamespace, "watch-namespace", "",
		"Comma-separated namespaces to watch for custom resources. Empty means all namespaces.")
	flag.IntVar(&maxConcurrentReconciles, "max-concurrent-reconciles", 1,
		"How many resources of one kind reconcile at once. Raise it when one slow app holds up the others.")
	opts := zap.Options{}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	enabled, err := resolveEnabledControllers(controllers, enableControllers, disableControllers)
	if err != nil {
		setupLog.Error(err, "Invalid controller selection")
		os.Exit(1)
	}

	// HTTP/2 is off by default because of the Stream Cancellation and Rapid
	// Reset CVEs (GHSA-qppj-fm5r-hxr3, GHSA-4374-p667-p6c8).
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("Disabling HTTP/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServerOptions := webhook.Options{
		TLSOpts: tlsOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	cacheOptions := cache.Options{DefaultNamespaces: watchedNamespaces(watchNamespace)}
	if len(cacheOptions.DefaultNamespaces) > 0 {
		setupLog.Info("Watching namespaces", "namespaces", watchNamespace)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "arrchitect.arrchitect.dev",
		Cache:                  cacheOptions,
		Client:                 ClientOptions(),
		Controller:             config.Controller{MaxConcurrentReconciles: maxConcurrentReconciles},
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	for _, c := range controllers {
		if !enabled[c.Name] {
			setupLog.Info("Controller disabled, skipping", "controller", c.Name)
			continue
		}
		reconciler := c.New(Deps{
			Client:   apiReaderClient{Client: mgr.GetClient(), reader: mgr.GetAPIReader()},
			Scheme:   mgr.GetScheme(),
			Recorder: mgr.GetEventRecorder(c.Name + "-controller"),
			Apps:     enabled,
		})
		if err := reconciler.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "Failed to create controller", "controller", c.Name)
			os.Exit(1)
		}
		setupLog.Info("Controller enabled", "controller", c.Name)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Failed to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "Failed to run manager")
		os.Exit(1)
	}
}

// ClientOptions read Secrets from the API server when they are used. The
// controllers watch Secrets by metadata only, so the manager keeps no copy of
// every Secret it can see.
func ClientOptions() client.Options {
	return client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}}
}

// watchedNamespaces turns --watch-namespace into the cache's namespaces. Nil
// watches every namespace.
func watchedNamespaces(flagValue string) map[string]cache.Config {
	var namespaces map[string]cache.Config
	for ns := range strings.SplitSeq(flagValue, ",") {
		if ns = strings.TrimSpace(ns); ns != "" {
			if namespaces == nil {
				namespaces = map[string]cache.Config{}
			}
			namespaces[ns] = cache.Config{}
		}
	}
	return namespaces
}

// resolveEnabledControllers computes which controllers are enabled based on the
// --enable-controllers and --disable-controllers flags. A name the manager
// doesn't run is an error rather than a controller silently left off.
func resolveEnabledControllers(controllers []Controller, enable, disable string) (map[string]bool, error) {
	known := make(map[string]bool, len(controllers))
	names := make([]string, 0, len(controllers))
	for _, c := range controllers {
		known[c.Name] = false
		names = append(names, c.Name)
	}
	set := func(list string, value bool) error {
		for name := range strings.SplitSeq(list, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if _, ok := known[name]; !ok {
				return fmt.Errorf("unknown controller %q; the controllers are %s", name, strings.Join(names, ", "))
			}
			known[name] = value
		}
		return nil
	}

	if strings.TrimSpace(enable) == "*" {
		for name := range known {
			known[name] = true
		}
	} else if err := set(enable, true); err != nil {
		return nil, err
	}
	if err := set(disable, false); err != nil {
		return nil, err
	}
	return known, nil
}
