package main

import (
	"context"
	"flag"
	golog "log"
	"net/http"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"k8s.io/apimachinery/pkg/util/wait"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	_ "github.com/openshift/generic-admission-server/pkg/cmd"

	cmdutil "github.com/openshift/hive/cmd/util"
	"github.com/openshift/hive/pkg/operator"
	"github.com/openshift/hive/pkg/operator/hive"
	utillogrus "github.com/openshift/hive/pkg/util/logrus"
	"github.com/openshift/hive/pkg/util/scheme"
	"github.com/openshift/hive/pkg/version"
)

const (
	defaultLogLevel        = "info"
	leaderElectionLockName = "hive-operator-leader"
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

				// Setup all Controllers
				if err := operator.AddToOperator(mgr); err != nil {
					log.Fatal(err)
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

			cmdutil.RunWithLeaderElection(ctx, cfg, operatorNS, leaderElectionLockName, run)
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
