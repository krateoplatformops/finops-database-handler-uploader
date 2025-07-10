package test

import (
	"context"
	"crypto/tls"
	"database-handler-uploader/api"
	v1 "database-handler-uploader/api/v1"
	uploadercontroller "database-handler-uploader/internal/controller"
	"database-handler-uploader/internal/helpers"
	"database-handler-uploader/internal/helpers/config"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/pkg/features"
	e2eutils "sigs.k8s.io/e2e-framework/pkg/utils"
	"sigs.k8s.io/e2e-framework/support/kind"

	"github.com/krateoplatformops/provider-runtime/pkg/controller"
	"github.com/krateoplatformops/provider-runtime/pkg/logging"
	"github.com/krateoplatformops/provider-runtime/pkg/ratelimiter"

	operatorlogger "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

type contextKey string

var (
	testenv env.Environment
)

const (
	testNamespace   = "uploader-test" // If you change this test namespace, you need to change in to_test as well
	crdsPath        = "../../crds"
	additionalCRDs  = "./manifests/crds"
	deploymentsPath = "./manifests/deployments"
	toTest          = "./manifests/to_test/"

	testName = "query"

	cratedbHost = "cratedb." + testNamespace
)

func TestMain(m *testing.M) {
	testenv = env.New()
	kindClusterName := "krateo-test"

	// Use pre-defined environment funcs to create a kind cluster prior to test run
	testenv.Setup(
		envfuncs.CreateCluster(kind.NewProvider(), kindClusterName),
		envfuncs.CreateNamespace(testNamespace),
		envfuncs.SetupCRDs(crdsPath, "*"),
		envfuncs.SetupCRDs(additionalCRDs, "*"),
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			// setup Krateo's helm
			if p := e2eutils.RunCommand("helm repo add krateo https://charts.krateo.io"); p.Err() != nil {
				return ctx, fmt.Errorf("helm error while adding repository: %s %v", p.Out(), p.Err())
			}

			if p := e2eutils.RunCommand("helm repo update krateo"); p.Err() != nil {
				return ctx, fmt.Errorf("helm error while updating helm: %s %v", p.Out(), p.Err())
			}

			// install cratedb-chart
			if p := e2eutils.RunCommand(
				fmt.Sprintf("helm install cratedb krateo/cratedb -n %s", testNamespace),
			); p.Err() != nil {
				return ctx, fmt.Errorf("helm error while installing chart: %s %v", p.Out(), p.Err())
			}

			// Wait for cratedb to install
			time.Sleep(45 * time.Second)

			// install finops-database-handler
			if p := e2eutils.RunCommand(
				fmt.Sprintf("helm install finops-database-handler krateo/finops-database-handler -n %s --set cratedbUserSystemName=%s --set env.CRATE_HOST=%s", testNamespace, "cratedb-system-credentials", cratedbHost),
			); p.Err() != nil {
				return ctx, fmt.Errorf("helm error while installing chart: %s %v", p.Out(), p.Err())
			}
			// Wait for finops-database-handler to install
			time.Sleep(25 * time.Second)

			log.Info().Msg("starting portforward")
			// open port
			go func() {
				log.Info().Msg("from goroutine: running port forward")
				_ = e2eutils.RunCommand(
					fmt.Sprintf("kubectl -n %s port-forward svc/%s 8088",
						testNamespace, "finops-database-handler"),
				)
			}()

			time.Sleep(10 * time.Second)

			return ctx, nil
		},
	)

	// Use pre-defined environment funcs to teardown kind cluster after tests
	testenv.Finish(
		envfuncs.DeleteNamespace(testNamespace),
		envfuncs.TeardownCRDs(crdsPath, "*"),
		func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
			// uninstall cratedb
			if p := e2eutils.RunCommand(
				fmt.Sprintf("helm uninstall cratedb -n %s", testNamespace),
			); p.Err() != nil {
				return ctx, fmt.Errorf("helm error while uninstalling chart: %s %v", p.Out(), p.Err())
			}

			// uninstall finops-database-handler
			if p := e2eutils.RunCommand(
				fmt.Sprintf("helm uninstall finops-database-handler -n %s", testNamespace),
			); p.Err() != nil {
				return ctx, fmt.Errorf("helm error while uninstalling chart: %s %v", p.Out(), p.Err())
			}
			return ctx, nil
		},
		envfuncs.DestroyCluster(kindClusterName),
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

func TestUploader(t *testing.T) {
	mgrCtx := t.Context()

	upload := features.New("Create").
		WithLabel("type", "Prepare").
		Setup(func(ctx context.Context, t *testing.T, c *envconf.Config) context.Context {
			r, err := resources.New(c.Client().RESTConfig())
			if err != nil {
				t.Fatal(err)
			}

			api.AddToScheme(r.GetScheme())
			r.WithNamespace(testNamespace)

			// Start the controller manager
			err = startTestManager(mgrCtx, r.GetScheme(), c.Client().RESTConfig())
			if err != nil {
				t.Fatal(err)
			}

			time.Sleep(10 * time.Second)

			ctx = context.WithValue(ctx, contextKey("client"), r)

			if err := wait.For(
				conditions.New(r).DeploymentAvailable("finops-database-handler", testNamespace),
				wait.WithTimeout(60*time.Second),
				wait.WithInterval(5*time.Second),
			); err != nil {
				log.Logger.Error().Err(err).Msg("Timed out while waiting for finops-database-handler deployment")
			}

			err = decoder.DecodeEachFile(
				ctx, os.DirFS(deploymentsPath), "*",
				decoder.CreateHandler(r),
				decoder.MutateNamespace(testNamespace),
			)
			if err != nil {
				t.Fatalf("Failed due to error: %s", err)
			}

			err = decoder.DecodeEachFile(
				ctx, os.DirFS(toTest), "*",
				decoder.CreateHandler(r),
				decoder.MutateNamespace(testNamespace),
			)
			if err != nil {
				t.Fatalf("Failed due to error: %s", err)
			}
			return ctx
		}).
		Assess("CRs", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			r := ctx.Value(contextKey("client")).(*resources.Resources)

			crGet := &v1.Notebook{}
			err := r.Get(ctx, testName+"1", testNamespace, crGet)
			if err != nil {
				t.Fatal(err)
			}

			err = r.Get(ctx, testName+"2", testNamespace, crGet)
			if err != nil {
				t.Fatal(err)
			}
			return ctx
		}).
		Assess("Remote value", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			r := ctx.Value(contextKey("client")).(*resources.Resources)

			// Get the secret first
			secret := &corev1.Secret{}
			err := r.Get(ctx, "cratedb-system-credentials", testNamespace, secret)
			if err != nil {
				t.Fatal(err)
			}

			time.Sleep(15 * time.Second)

			log.Debug().Msgf("curl -u 'system:%s' -s http://%s:%s/compute/%s1/info", string(secret.Data["password"]), "localhost", "8088", testName)

			p := e2eutils.RunCommand(fmt.Sprintf("curl -u 'system:%s' -s --max-time 30 http://%s:%s/compute/%s1/info", string(secret.Data["password"]), "localhost", "8088", testName))
			if p.Err() != nil {
				t.Fatal(fmt.Errorf("error with curl: %s %v", p.Out(), p.Err()))
			}
			resultData1, _ := io.ReadAll(p.Out())
			log.Info().Msgf("resultData1: %s", string(resultData1))
			var notebookValues1 helpers.NotebookInfo
			err = json.Unmarshal(resultData1, &notebookValues1)
			if err != nil {
				t.Fatal(fmt.Errorf("could not unmarshal notebook code from finops-database-handler: %v", err))
			}

			log.Debug().Msgf("curl -u 'system:%s' -s http://%s:%s/compute/%s2/info", string(secret.Data["password"]), "localhost", "8088", testName)

			p = e2eutils.RunCommand(fmt.Sprintf("curl -u 'system:%s' -s --max-time 30 http://%s:%s/compute/%s2/info", string(secret.Data["password"]), "localhost", "8088", testName))
			if p.Err() != nil {
				t.Fatal(fmt.Errorf("error with curl: %s %v", p.Out(), p.Err()))
			}
			resultData2, _ := io.ReadAll(p.Out())
			log.Info().Msgf("resultData2: %s", string(resultData2))
			var notebookValues2 helpers.NotebookInfo
			err = json.Unmarshal(resultData2, &notebookValues2)
			if err != nil {
				t.Fatal(fmt.Errorf("could not unmarshal notebook code from finops-database-handler: %v", err))
			}

			if strings.ReplaceAll(expectedOutput, "\n", "") != strings.ReplaceAll(notebookValues1.Code, "\n", "") || strings.ReplaceAll(expectedOutput, "\n", "") != strings.ReplaceAll(notebookValues2.Code, "\n", "") {
				log.Warn().Msgf("expected: %s", expectedOutput)
				log.Warn().Msgf("notebook1: %s", notebookValues1.Code)
				log.Warn().Msgf("notebook2: %s", notebookValues2.Code)
				t.Fatal(fmt.Errorf("Unexpected output from finops-database-handler"))
			}

			return ctx
		}).
		Feature()

	// test feature
	testenv.Test(t, upload)
}

// startTestManager starts the controller manager with the given config
func startTestManager(ctx context.Context, scheme *runtime.Scheme, cfg *rest.Config) error {
	os.Setenv("FINOPS_DATABASE_HANDLER_ENDPOINT_NAMESPACE", testNamespace)
	os.Setenv("FINOPS_DATABASE_HANDLER_URL_OVERRIDE", "http://localhost:8088")

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
	log.Logger.Info().Msgf("config %s", configuration.String())

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	disableHTTP2 := func(c *tls.Config) {
		c.NextProtos = []string{"http/1.1"}
	}

	tlsOpts := []func(*tls.Config){}
	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	namespaceCacheConfigMap := make(map[string]cache.Config)
	namespaceCacheConfigMap[testNamespace] = cache.Config{}
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
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

	pollingIntervalString := configuration.PollingInterval
	maxReconcileRateString := configuration.MaxReconcileRate

	pollingIntervalInt, err := strconv.Atoi(pollingIntervalString)
	pollingInterval := time.Duration(time.Duration(0))

	if err == nil {
		pollingInterval = time.Duration(pollingIntervalInt) * time.Second
	}

	maxReconcileRate, err := strconv.Atoi(maxReconcileRateString)
	if err != nil {
		maxReconcileRate = 1
	}

	o := controller.Options{
		Logger:                  logging.NewLogrLogger(operatorlogger.Log.WithName("database-handler-uploader")),
		MaxConcurrentReconciles: maxReconcileRate,
		PollInterval:            pollingInterval,
		GlobalRateLimiter:       ratelimiter.NewGlobal(maxReconcileRate),
	}

	if err := uploadercontroller.Setup(mgr, o, configuration); err != nil {
		return err
	}

	go func() {
		if err := mgr.Start(ctx); err != nil {
			log.Error().Err(err).Msg("Failed to start manager")
		}
	}()

	return nil
}

const expectedOutput = "# The notebook is injected with additional lines of code:\n# import sys\n# from crate import client\n# def eprint(*args, **kwargs):\n#     print(*args, file=sys.stderr, **kwargs)\n# host = sys.argv[1]\n# port = sys.argv[2]\n# username = sys.argv[3]\n# password = sys.argv[4]\n# try:\n#     connection = client.connect(f\"http://{host}:{port}\", username=username, password=password)\n#     cursor = connection.cursor()\n# except Exception as e:\n#     eprint('error while connecting to database' + str(e))\n#     raise\n\nimport pip._internal as pip\ndef install(package):\n    pip.main(['install', package])\n\ndef main():   \n    table_name_arg = sys.argv[5]\n    table_name_key_value = str.split(table_name_arg, '=')\n    if len(table_name_key_value) == 2:\n        if table_name_key_value[0] == 'table_name':\n            table_name = table_name_key_value[1]\n    try:\n        resource_query = f\"SELECT * FROM {table_name}\"\n        cursor.execute(resource_query)\n        raw_data = cursor.fetchall()\n        print(pd.DataFrame(raw_data))\n    finally:\n        cursor.close()\n        connection.close()\n\n\nif __name__ == \"__main__\":\n    try:\n        import pandas as pd\n    except ImportError:\n        install('pandas')\n        import pandas as pd\n    main()\n"
