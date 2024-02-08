package main

import (
	"context"
	"flag"
	golog "log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	_ "github.com/openshift/generic-admission-server/pkg/cmd"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	cmdutil "github.com/openshift/hive/cmd/util"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/argocdregister"
	"github.com/openshift/hive/pkg/controller/awsprivatelink"
	"github.com/openshift/hive/pkg/controller/clusterclaim"
	"github.com/openshift/hive/pkg/controller/clusterdeployment"
	"github.com/openshift/hive/pkg/controller/clusterdeprovision"
	"github.com/openshift/hive/pkg/controller/clusterpool"
	"github.com/openshift/hive/pkg/controller/clusterpoolnamespace"
	"github.com/openshift/hive/pkg/controller/clusterprovision"
	"github.com/openshift/hive/pkg/controller/clusterrelocate"
	"github.com/openshift/hive/pkg/controller/clusterstate"
	"github.com/openshift/hive/pkg/controller/clustersync"
	"github.com/openshift/hive/pkg/controller/clusterversion"
	"github.com/openshift/hive/pkg/controller/controlplanecerts"
	"github.com/openshift/hive/pkg/controller/dnsendpoint"
	"github.com/openshift/hive/pkg/controller/dnszone"
	"github.com/openshift/hive/pkg/controller/fakeclusterinstall"
	"github.com/openshift/hive/pkg/controller/hibernation"
	"github.com/openshift/hive/pkg/controller/machinepool"
	"github.com/openshift/hive/pkg/controller/metrics"
	"github.com/openshift/hive/pkg/controller/remoteingress"
	"github.com/openshift/hive/pkg/controller/syncidentityprovider"
	"github.com/openshift/hive/pkg/controller/unreachable"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/controller/velerobackup"
	utillogrus "github.com/openshift/hive/pkg/util/logrus"
	"github.com/openshift/hive/pkg/util/scheme"
	"github.com/openshift/hive/pkg/version"
)

const (
	defaultLogLevel        = "info"
	leaderElectionLockName = "hive-controllers-leader"
	// Matches the `port` in config/{controllers|clustersync}/service.yaml
	pprofHostPort = "localhost:6060"
)

type controllerSetupFunc func(manager.Manager) error

var controllerFuncs = map[hivev1.ControllerName]controllerSetupFunc{
	clusterclaim.ControllerName:         clusterclaim.Add,
	clusterdeployment.ControllerName:    clusterdeployment.Add,
	clusterdeprovision.ControllerName:   clusterdeprovision.Add,
	clusterpoolnamespace.ControllerName: clusterpoolnamespace.Add,
	clusterprovision.ControllerName:     clusterprovision.Add,
	clusterrelocate.ControllerName:      clusterrelocate.Add,
	clusterstate.ControllerName:         clusterstate.Add,
	clustersync.ControllerName:          clustersync.Add,
	clusterversion.ControllerName:       clusterversion.Add,
	controlplanecerts.ControllerName:    controlplanecerts.Add,
	dnsendpoint.ControllerName:          dnsendpoint.Add,
	dnszone.ControllerName:              dnszone.Add,
	fakeclusterinstall.ControllerName:   fakeclusterinstall.Add,
	metrics.ControllerName:              metrics.Add,
	remoteingress.ControllerName:        remoteingress.Add,
	machinepool.ControllerName:          machinepool.Add,
	syncidentityprovider.ControllerName: syncidentityprovider.Add,
	unreachable.ControllerName:          unreachable.Add,
	velerobackup.ControllerName:         velerobackup.Add,
	clusterpool.ControllerName:          clusterpool.Add,
	hibernation.ControllerName:          hibernation.Add,
	awsprivatelink.ControllerName:       awsprivatelink.Add,
	argocdregister.ControllerName:       argocdregister.Add,
}

// disabledControllerEquivalents contains a mapping of old controller names to their new equivalent so that CLI parameters like --controllers and --disabled-controllers continue to work
var disabledControllerEquivalents = map[string]string{
	// RemoteMachineSet controller was renamed to MachinePool controller.
	hivev1.MachinePoolControllerName.String(): hivev1.DeprecatedRemoteMachinesetControllerName.String(),
}

type controllerManagerOptions struct {
	LogLevel            string
	Controllers         []string
	DisabledControllers []string
}

func newRootCommand() *cobra.Command {
	opts := newControllerManagerOptions()
	cmd := &cobra.Command{
		Use:   "manager",
		Short: "OpenShift Hive controller manager.",
		Run: func(cmd *cobra.Command, args []string) {
			// Set log level
			level, err := log.ParseLevel(opts.LogLevel)
			if err != nil {
				log.WithError(err).Fatal("Cannot parse log level")
			}
			log.SetLevel(level)

			// Add some millisecond precision to log timestamps, useful for debugging performance.
			formatter := new(log.TextFormatter)
			formatter.TimestampFormat = "2006-01-02T15:04:05.999Z07:00"
			formatter.FullTimestamp = true
			log.SetFormatter(formatter)

			log.Infof("Version: %s", version.String())
			log.Debug("debug logging enabled")

			// Get a config to talk to the apiserver
			cfg, err := config.GetConfig()
			if err != nil {
				log.Fatal(err)
			}

			if os.Getenv(constants.HiveNamespaceEnvVar) == "" {
				log.Warnf("%s env var is not defined, using default: %s", constants.HiveNamespaceEnvVar,
					constants.DefaultHiveNamespace)
			}
			hiveNSName := utils.GetHiveNamespace()
			log.Infof("hive namespace: %s", hiveNSName)

			// Create and start liveness and readiness probe endpoints
			http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			http.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			log.Info("Starting /healthz and /readyz endpoints")
			go http.ListenAndServe(":8080", nil)

			// use a Go context so we can tell the leaderelection code when we want to step down
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			run := func(ctx context.Context) {
				// Create a new Cmd to provide shared dependencies and start components
				mgr, err := manager.New(cfg, manager.Options{
					Scheme:  scheme.GetScheme(),
					Metrics: metricsserver.Options{BindAddress: ":2112"},
					Logger:  utillogrus.NewLogr(log.StandardLogger()),
				})
				if err != nil {
					log.Fatal(err)
				}

				log.Info("Registering Components.")

				if err := utils.SetupAdditionalCA(); err != nil {
					log.Fatal(err)
				}

				disabledControllersSet := sets.NewString(opts.DisabledControllers...)
				// Setup all Controllers
				for _, name := range opts.Controllers {
					fn, ok := controllerFuncs[hivev1.ControllerName(name)]
					if !ok {
						log.WithField("controller", name).Fatal("no entry for controller found")
					}
					if disabledControllersSet.Has(name) {
						log.WithField("controller", name).Debugf("skipping disabled controller")
						continue
					}
					disabledEquivalent := disabledControllerEquivalents[name]
					if disabledEquivalent != "" && disabledControllersSet.Has(disabledEquivalent) {
						log.WithField("controller", name).Debugf("skipping disabled controller because %s has been disabled", disabledEquivalent)
						continue
					}
					if err := fn(mgr); err != nil {
						log.WithError(err).WithField("controller", name).Fatal("failed to start controller")
					}
				}

				log.Info("Starting the Cmd.")

				// Start the Cmd
				err = mgr.Start(signals.SetupSignalHandler())
				if err != nil {
					log.WithError(err).Error("error running manager")
				}
				// Canceling the leader election context
				cancel()
			}

			if os.Getenv("HIVE_SKIP_LEADER_ELECTION") != "" {
				run(ctx)
			} else {
				cmdutil.RunWithLeaderElection(ctx, cfg, hiveNSName, leaderElectionLockName, run)
			}
		},
	}

	cmd.PersistentFlags().StringVar(&opts.LogLevel, "log-level", defaultLogLevel, "Log level (debug,info,warn,error,fatal)")
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	cmd.PersistentFlags().StringSliceVar(&opts.Controllers, "controllers", opts.Controllers, "Comma-separated list of controllers to run")
	cmd.PersistentFlags().StringSliceVar(&opts.DisabledControllers, "disabled-controllers", []string{},
		"Comma-separated list of controllers to disable (overrides anything enabled with the --controllers param)")
	initializeKlog(cmd.PersistentFlags())
	flag.CommandLine.Parse([]string{})

	return cmd
}

func newControllerManagerOptions() *controllerManagerOptions {
	// By default we have all of the controllers enabled
	controllers := make([]string, 0, len(controllerFuncs))
	for name := range controllerFuncs {
		controllers = append(controllers, name.String())
	}
	return &controllerManagerOptions{
		Controllers: controllers,
	}
}

func initializeKlog(flags *pflag.FlagSet) {
	golog.SetOutput(klogWriter{}) // Redirect all regular go log output to klog
	golog.SetFlags(0)

	go wait.Forever(klog.Flush, 5*time.Second) // Periodically flush logs
	f := flags.Lookup("logtostderr")           // Default to logging to stderr
	if f != nil {
		f.Value.Set("true")
	}
}

type klogWriter struct{}

func (writer klogWriter) Write(data []byte) (n int, err error) {
	klog.Info(string(data))
	return len(data), nil
}

func main() {
	defer klog.Flush()
	go func() {
		log.WithField("pprof_host_port", pprofHostPort).Info("Enabling pprof")
		log.Println(http.ListenAndServe(pprofHostPort, nil))
	}()
	rand.Seed(time.Now().UnixNano())
	cmd := newRootCommand()
	err := cmd.Execute()
	if err != nil {
		log.Fatal(err)
	}
}
