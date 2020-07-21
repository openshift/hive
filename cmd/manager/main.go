package main

import (
	"flag"
	golog "log"
	"math/rand"
	"os"
	"strconv"
	"time"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	crv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"k8s.io/klog"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	openshiftapiv1 "github.com/openshift/api/config/v1"
	_ "github.com/openshift/generic-admission-server/pkg/cmd"

	"github.com/openshift/hive/pkg/apis"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/controller/clusterdeployment"
	"github.com/openshift/hive/pkg/controller/clusterdeprovision"
	"github.com/openshift/hive/pkg/controller/clusterpoolnamespace"
	"github.com/openshift/hive/pkg/controller/clusterprovision"
	"github.com/openshift/hive/pkg/controller/clusterrelocate"
	"github.com/openshift/hive/pkg/controller/clusterstate"
	"github.com/openshift/hive/pkg/controller/clusterversion"
	"github.com/openshift/hive/pkg/controller/controlplanecerts"
	"github.com/openshift/hive/pkg/controller/dnsendpoint"
	"github.com/openshift/hive/pkg/controller/dnszone"
	"github.com/openshift/hive/pkg/controller/metrics"
	"github.com/openshift/hive/pkg/controller/remoteingress"
	"github.com/openshift/hive/pkg/controller/remotemachineset"
	"github.com/openshift/hive/pkg/controller/syncidentityprovider"
	"github.com/openshift/hive/pkg/controller/syncset"
	"github.com/openshift/hive/pkg/controller/syncsetinstance"
	"github.com/openshift/hive/pkg/controller/unreachable"
	"github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/controller/velerobackup"
	"github.com/openshift/hive/pkg/version"
)

const (
	defaultLogLevel             = "info"
	leaderElectionConfigMap     = "hive-controllers-leader"
	leaderElectionLeaseDuration = "40s"
	leaderElectionRenewDeadline = "20s"
	leaderElectionlRetryPeriod  = "4s"
)

type controllerSetupFunc func(manager.Manager) error

var controllerFuncs = map[string]controllerSetupFunc{
	clusterdeployment.ControllerName:    clusterdeployment.Add,
	clusterdeprovision.ControllerName:   clusterdeprovision.Add,
	clusterpoolnamespace.ControllerName: clusterpoolnamespace.Add,
	clusterprovision.ControllerName:     clusterprovision.Add,
	clusterrelocate.ControllerName:      clusterrelocate.Add,
	clusterstate.ControllerName:         clusterstate.Add,
	clusterversion.ControllerName:       clusterversion.Add,
	controlplanecerts.ControllerName:    controlplanecerts.Add,
	dnsendpoint.ControllerName:          dnsendpoint.Add,
	dnszone.ControllerName:              dnszone.Add,
	metrics.ControllerName:              metrics.Add,
	remoteingress.ControllerName:        remoteingress.Add,
	remotemachineset.ControllerName:     remotemachineset.Add,
	syncidentityprovider.ControllerName: syncidentityprovider.Add,
	syncset.ControllerName:              syncset.Add,
	syncsetinstance.ControllerName:      syncsetinstance.Add,
	unreachable.ControllerName:          unreachable.Add,
	velerobackup.ControllerName:         velerobackup.Add,
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
			log.Infof("Version: %s", version.String())
			log.Debug("debug logging enabled")

			// Parse leader election options
			leaseDuration, err := time.ParseDuration(leaderElectionLeaseDuration)
			if err != nil {
				log.WithError(err).Fatal("Cannot parse lease duration")
			}
			renewDeadline, err := time.ParseDuration(leaderElectionRenewDeadline)
			if err != nil {
				log.WithError(err).Fatal("Cannot parse renew deadline")
			}
			retryPeriod, err := time.ParseDuration(leaderElectionlRetryPeriod)
			if err != nil {
				log.WithError(err).Fatal("Cannot parse retry period")
			}

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

			// Allow an env var to disable leader election, useful when testing from source with make run to eliminate the wait time.
			var disableLeaderElection bool
			val := os.Getenv("DISABLE_LEADER_ELECTION")
			if val != "" {
				disableLeaderElection, err = strconv.ParseBool(val)
				if err != nil {
					log.WithField("value", val).Fatal("Error parsing DISABLE_LEADER_ELECTION env var")
				}
			}

			// Create a new Cmd to provide shared dependencies and start components
			mgr, err := manager.New(cfg, manager.Options{
				LeaderElection:          !disableLeaderElection,
				LeaderElectionNamespace: hiveNSName,
				LeaderElectionID:        leaderElectionConfigMap,
				LeaseDuration:           &leaseDuration,
				RenewDeadline:           &renewDeadline,
				RetryPeriod:             &retryPeriod,
				MetricsBindAddress:      ":2112",
				HealthProbeBindAddress:  ":8080",
			})
			if err != nil {
				log.Fatal(err)
			}

			log.Info("Registering Components.")

			if err := utils.SetupAdditionalCA(); err != nil {
				log.Fatal(err)
			}

			// Setup Scheme for all resources
			if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := openshiftapiv1.Install(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := apiextv1.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := crv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := velerov1.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := awsprovider.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			disabledControllersSet := sets.NewString(opts.DisabledControllers...)
			// Setup all Controllers
			for _, name := range opts.Controllers {
				fn, ok := controllerFuncs[name]
				if !ok {
					log.WithField("controller", name).Fatal("no entry for controller found")
				}
				if disabledControllersSet.Has(name) {
					log.WithField("controller", name).Debugf("skipping disabled controller")
					continue
				}
				if err := fn(mgr); err != nil {
					log.WithError(err).WithField("controller", name).Fatal("failed to start controller")
				}
			}

			mgr.AddReadyzCheck("ping", healthz.Ping)
			mgr.AddHealthzCheck("ping", healthz.Ping)

			log.Info("Starting the Cmd.")

			// Start the Cmd
			log.Fatal(mgr.Start(signals.SetupSignalHandler()))
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
		controllers = append(controllers, name)
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
	rand.Seed(time.Now().UnixNano())
	cmd := newRootCommand()
	err := cmd.Execute()
	if err != nil {
		log.Fatal(err)
	}
}
