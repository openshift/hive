package main

import (
	"context"
	"flag"
	golog "log"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/klog"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	oappsv1 "github.com/openshift/api/apps/v1"
	orbacv1 "github.com/openshift/api/authorization/v1"
	_ "github.com/openshift/generic-admission-server/pkg/cmd"

	"github.com/openshift/hive/apis"
	"github.com/openshift/hive/pkg/operator"
	"github.com/openshift/hive/pkg/operator/hive"
	utillogrus "github.com/openshift/hive/pkg/util/logrus"
	"github.com/openshift/hive/pkg/version"
)

const (
	defaultLogLevel             = "info"
	leaderElectionConfigMap     = "hive-operator-leader"
	leaderElectionLeaseDuration = "120s"
	leaderElectionRenewDeadline = "90s"
	leaderElectionRetryPeriod   = "30s"
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
			retryPeriod, err := time.ParseDuration(leaderElectionRetryPeriod)
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
					MetricsBindAddress: ":2112",
					Logger:             utillogrus.NewLogr(log.StandardLogger()),
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

				if err := apiextv1.AddToScheme(mgr.GetScheme()); err != nil {
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

				log.Info("Starting the Cmd.")

				// Start the Cmd
				err = mgr.Start(signals.SetupSignalHandler())
				if err != nil {
					log.WithError(err).Error("error running manager")
				}
				// Canceling the leader election context
				cancel()
			}

			// Leader election code based on:
			// https://github.com/kubernetes/kubernetes/blob/f7e3bcdec2e090b7361a61e21c20b3dbbb41b7f0/staging/src/k8s.io/client-go/examples/leader-election/main.go#L92-L154
			// This gives us ReleaseOnCancel which is not presently exposed in controller-runtime.

			id := uuid.New().String()
			leLog := log.WithField("id", id)
			leLog.Info("generated leader election ID")

			lock := &resourcelock.ConfigMapLock{
				ConfigMapMeta: metav1.ObjectMeta{
					Namespace: operatorNS,
					Name:      leaderElectionConfigMap,
				},
				Client: kubernetes.NewForConfigOrDie(cfg).CoreV1(),
				LockConfig: resourcelock.ResourceLockConfig{
					Identity: id,
				},
			}

			// start the leader election code loop
			leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
				Lock:            lock,
				ReleaseOnCancel: true,
				LeaseDuration:   leaseDuration,
				RenewDeadline:   renewDeadline,
				RetryPeriod:     retryPeriod,
				Callbacks: leaderelection.LeaderCallbacks{
					OnStartedLeading: func(ctx context.Context) {
						run(ctx)
					},
					OnStoppedLeading: func() {
						// we can do cleanup here if necessary
						leLog.Infof("leader lost")
						os.Exit(0)
					},
					OnNewLeader: func(identity string) {
						if identity == id {
							// We just became the leader
							leLog.Info("became leader")
							return
						}
						log.Infof("current leader: %s", identity)
					},
				},
			})

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
