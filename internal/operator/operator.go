// Package operator runs one media-operator binary: its flags, manager and
// controllers.
package operator

import (
	"crypto/tls"
	"flag"
	"os"
	"strings"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

// Deps are what a reconciler is built from: the manager's client and scheme,
// and an event recorder named for the controller.
type Deps struct {
	Client   client.Client
	Scheme   *runtime.Scheme
	Recorder events.EventRecorder
}

// Controller is one integration a binary runs.
type Controller struct {
	Name string
	New  func(Deps) Reconciler
}

var setupLog = ctrl.Log.WithName("setup")

// Run parses the flags, starts the manager for the named binary with the
// given controllers, and blocks until it stops. It exits the process on error.
func Run(binary string, addToScheme func(*runtime.Scheme) error, controllers ...Controller) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(addToScheme(scheme))

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
		"Namespace to watch for custom resources. Empty means all namespaces.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
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

	cacheOptions := cache.Options{}
	if watchNamespace != "" {
		cacheOptions.DefaultNamespaces = map[string]cache.Config{watchNamespace: {}}
		setupLog.Info("Watching a single namespace", "namespace", watchNamespace)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "media-operator-" + binary + ".media-operator.dev",
		Cache:                  cacheOptions,
	})
	if err != nil {
		setupLog.Error(err, "Failed to start manager")
		os.Exit(1)
	}

	enabled := resolveEnabledControllers(controllers, enableControllers, disableControllers)

	for _, c := range controllers {
		if !enabled[c.Name] {
			setupLog.Info("Controller disabled, skipping", "controller", c.Name)
			continue
		}
		reconciler := c.New(Deps{Client: mgr.GetClient(), Scheme: mgr.GetScheme(), Recorder: mgr.GetEventRecorder(c.Name + "-controller")})
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

// resolveEnabledControllers computes which controllers are enabled based on the
// --enable-controllers and --disable-controllers flags.
func resolveEnabledControllers(controllers []Controller, enable, disable string) map[string]bool {
	known := make(map[string]bool, len(controllers))
	for _, c := range controllers {
		known[c.Name] = false
	}

	if strings.TrimSpace(enable) == "*" {
		for name := range known {
			known[name] = true
		}
	} else {
		for name := range strings.SplitSeq(enable, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				known[name] = true
			}
		}
	}

	for name := range strings.SplitSeq(disable, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			known[name] = false
		}
	}

	return known
}
