package main

import (
	"flag"
	golog "log"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	oappsv1 "github.com/openshift/api/apps/v1"
	orbacv1 "github.com/openshift/api/authorization/v1"
	_ "github.com/openshift/generic-admission-server/pkg/cmd"

	"github.com/openshift/hive/pkg/apis"
	"github.com/openshift/hive/pkg/operator"
	"github.com/openshift/hive/pkg/operator/hive"
	"github.com/openshift/hive/pkg/version"
)

const (
	defaultLogLevel             = "info"
	leaderElectionConfigMap     = "hive-operator-leader"
	leaderElectionLeaseDuration = "40s"
	leaderElectionRenewDeadline = "20s"
	leaderElectionlRetryPeriod  = "4s"
)

type controllerManagerOptions struct {
	LogLevel string
}

func newRootCommand() *cobra.Command {
	opts := &controllerManagerOptions{}
	cmd := &cobra.Command{
		Use:   "hive-operator",
		Short: "OpenShift Hive Operator",
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

			// We must be provided an env var indicating where the hive-operator is running. Normally
			// passed by the hive-operator Deployment using the kube downward API.
			operatorNS := os.Getenv(hive.HiveOperatorNamespaceEnvVar)
			if operatorNS == "" {
				log.Fatalf("%s env var is unset, unable to determine namespace operator is running in", hive.HiveOperatorNamespaceEnvVar)
			}

			// Create a new Cmd to provide shared dependencies and start components
			mgr, err := manager.New(cfg, manager.Options{
				LeaderElectionNamespace: operatorNS,
				LeaderElection:          true,
				LeaderElectionID:        leaderElectionConfigMap,
				LeaseDuration:           &leaseDuration,
				RenewDeadline:           &renewDeadline,
				RetryPeriod:             &retryPeriod,
				MetricsBindAddress:      "0",
				HealthProbeBindAddress:  ":8080",
			})
			if err != nil {
				log.Fatal(err)
			}

			log.Info("Registering Components.")

			// Setup Scheme for all resources
			if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := apiregistrationv1.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := apiextv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := oappsv1.Install(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			if err := orbacv1.Install(mgr.GetScheme()); err != nil {
				log.Fatal(err)
			}

			// Setup all Controllers
			if err := operator.AddToOperator(mgr); err != nil {
				log.Fatal(err)
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
	initializeKlog(cmd.PersistentFlags())
	flag.CommandLine.Parse([]string{})

	return cmd
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
	cmd := newRootCommand()
	err := cmd.Execute()
	if err != nil {
		log.Fatal(err)
	}
}
