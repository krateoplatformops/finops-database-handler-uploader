package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/krateoplatformops/provider-runtime/pkg/controller"
	"github.com/krateoplatformops/provider-runtime/pkg/logging"
	"github.com/krateoplatformops/provider-runtime/pkg/ratelimiter"

	uploadercontroller "database-handler-uploader/internal/controller"
	"database-handler-uploader/internal/helpers/config"

	api "database-handler-uploader/api/v1"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(api.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", false,
		"If set the metrics endpoint is served securely")
	flag.BoolVar(&enableHTTP2, "enable-http2", false,
		"If set, HTTP/2 will be enabled for the metrics and webhook servers")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	configuration := config.ParseConfig()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	watchNamespace := configuration.WatchNamespace
	namespaceCacheConfigMap := make(map[string]cache.Config)
	namespaceCacheConfigMap[watchNamespace] = cache.Config{}
	if watchNamespace == "" {
		setupLog.Error(fmt.Errorf("WATCH_NAMESPACE is empty"), "unable to get WatchNamespace")
		os.Exit(1)
	}

	pollingIntervalString := configuration.PollingInterval
	maxReconcileRateString := configuration.MaxReconcileRate

	pollingIntervalInt, err := strconv.Atoi(pollingIntervalString)
	pollingInterval := time.Duration(time.Duration(0))

	if err != nil {
		setupLog.Error(err, "unable to parse POLLING_INTERVAL, using default value")
	} else if pollingIntervalInt != 0 {
		pollingInterval = time.Duration(pollingIntervalInt) * time.Second
	}

	maxReconcileRate, err := strconv.Atoi(maxReconcileRateString)
	if err != nil {
		setupLog.Error(err, "unable to parse MAX_RECONCILE_RATE, using default value (1)")
		maxReconcileRate = 1
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress:   metricsAddr,
			SecureServing: secureMetrics,
			TLSOpts:       tlsOpts,
		},
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "9s8d9938.krateo.io",
		Cache:                  cache.Options{DefaultNamespaces: namespaceCacheConfigMap},
	})
	if err != nil {
		os.Exit(1)
	}

	o := controller.Options{
		Logger:                  logging.NewLogrLogger(log.Log.WithName("database-handler-uploader")),
		MaxConcurrentReconciles: maxReconcileRate,
		PollInterval:            pollingInterval,
		GlobalRateLimiter:       ratelimiter.NewGlobal(maxReconcileRate),
	}

	if err := uploadercontroller.Setup(mgr, o, configuration); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CompositionReference")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
