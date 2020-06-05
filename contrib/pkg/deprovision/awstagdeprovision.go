package deprovision

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/installer/pkg/destroy/aws"
	"github.com/openshift/library-go/pkg/controller/fileobserver"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

type awsTagDeprovisionOpts struct {
	logLevel           string
	region             string
	filters            []aws.Filter
	clusterDeprovision string
	logger             log.FieldLogger
}

// NewDeprovisionAWSWithTagsCommand is the entrypoint to create the 'aws-tag-deprovision' subcommand
// TODO: Port to a sub-command of deprovision.
func NewDeprovisionAWSWithTagsCommand() *cobra.Command {
	opt := &awsTagDeprovisionOpts{}
	cmd := &cobra.Command{
		Use:   "aws-tag-deprovision KEY=VALUE ...",
		Short: "Deprovision AWS assets (as created by openshift-installer) with the given tag(s)",
		Long:  "Deprovision AWS assets (as created by openshift-installer) with the given tag(s).  A resource matches the filter if any of the key/value pairs are in its tags.",
		Run: func(cmd *cobra.Command, args []string) {
			if err := opt.complete(args); err != nil {
				log.WithError(err).Error("Cannot complete command")
				return
			}

			// If environment variables are not set, assume we're running in a deprovision pod with an aws credentials secret mounted.
			// Parse credentials from files mounted in as a secret volume and set as env vars. We use the file
			// instead of passing env vars directly so we can monitor for changes and restart the pod if an admin
			// modifies the creds secret after the uninstall job has launched.
			if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_SECRET_ACCESS_KEY") == "" {
				log.WithField("credsMount", constants.AWSCredsMount).Info(
					"AWS environment variables not set, assume running in pod with cred secret mounted")
				awsAccessKeyIDFile := filepath.Join(constants.AWSCredsMount, constants.AWSAccessKeyIDSecretKey)
				awsSecretAccessKeyFile := filepath.Join(constants.AWSCredsMount, constants.AWSSecretAccessKeySecretKey)
				files := make(map[string][]byte, 2)

				data, err := ioutil.ReadFile(awsAccessKeyIDFile)
				if err != nil {
					log.WithError(err).Fatalf("error reading %s file", awsAccessKeyIDFile)
				}
				os.Setenv("AWS_ACCESS_KEY_ID", string(data))
				files[awsAccessKeyIDFile] = data

				data, err = ioutil.ReadFile(awsSecretAccessKeyFile)
				if err != nil {
					log.WithError(err).Fatalf("error reading %s file", awsSecretAccessKeyFile)
				}
				os.Setenv("AWS_SECRET_ACCESS_KEY", string(data))
				files[awsSecretAccessKeyFile] = data

				fullFilenames := []string{
					awsAccessKeyIDFile,
					awsSecretAccessKeyFile,
				}
				obs, err := fileobserver.NewObserver(10 * time.Second)
				if err != nil {
					log.WithError(err).Fatal("could not set up file observer to check for credentials changes")
				}
				obs.AddReactor(func(filename string, action fileobserver.ActionType) error {
					log.WithField("filename", filename).Info("exiting because credentials secret mount file changed")
					os.Exit(1)
					return nil
				}, files, fullFilenames...)

				stop := make(chan struct{})
				go func() {
					log.WithField("files", fullFilenames).Info("running file observer")
					obs.Run(stop)
					log.Fatal("file observer stopped")
				}()
			}

			if err := opt.run(); err != nil {
				log.WithError(err).Fatal("Runtime error")
			}
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opt.logLevel, "loglevel", "info", "log level, one of: debug, info, warn, error, fatal, panic")
	flags.StringVar(&opt.region, "region", "us-east-1", "AWS region to use")
	flags.StringVar(&opt.clusterDeprovision, "clusterdeprovision", "", "name of the ClusterDeprovision in which to stored blocked resources")
	return cmd
}

func (o *awsTagDeprovisionOpts) complete(args []string) error {
	for _, arg := range args {
		filter := aws.Filter{}
		err := parseFilter(filter, arg)
		if err != nil {
			return fmt.Errorf("cannot parse filter %s: %v", arg, err)
		}
		o.filters = append(o.filters, filter)
	}

	// Set log level
	level, err := log.ParseLevel(o.logLevel)
	if err != nil {
		log.WithError(err).Error("cannot parse log level")
		return err
	}

	o.logger = log.NewEntry(&log.Logger{
		Out: os.Stdout,
		Formatter: &log.TextFormatter{
			FullTimestamp: true,
		},
		Hooks: make(log.LevelHooks),
		Level: level,
	})

	return nil
}

func (o *awsTagDeprovisionOpts) run() error {
	return wait.ExponentialBackoff(
		// Start the backoff at 5 minutes, double it each time, to a cap of 24 hours.
		wait.Backoff{
			Duration: 5 * time.Minute,
			Factor:   2,
			Steps:    1 << 8, // large enough to make cap the effective bound
			Cap:      24 * time.Hour,
		},
		func() (done bool, returnErr error) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			uninstaller := &aws.ClusterUninstaller{
				Filters: o.filters,
				Logger:  o.logger,
				Region:  o.region,
			}
			blockedResources, err := uninstaller.RunWithContext(ctx)
			if len(blockedResources) == 0 {
				return true, err
			}
			if o.clusterDeprovision == "" {
				return
			}
			kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
				clientcmd.NewDefaultClientConfigLoadingRules(),
				&clientcmd.ConfigOverrides{},
			)
			namespace, _, err := kubeconfig.Namespace()
			if err != nil {
				o.logger.WithError(err).Error("could not get the namespace")
				return
			}
			clientConfig, err := kubeconfig.ClientConfig()
			if err != nil {
				o.logger.WithError(err).Error("could not get the kube client config")
				return
			}
			scheme := runtime.NewScheme()
			hivev1.AddToScheme(scheme)
			c, err := client.New(clientConfig, client.Options{Scheme: scheme})
			if err != nil {
				o.logger.WithError(err).Error("could not get kube client")
				return
			}
			clusterDeprovision := &hivev1.ClusterDeprovision{}
			if err := c.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: o.clusterDeprovision}, clusterDeprovision); err != nil {
				o.logger.WithError(err).Error("could not get ClusterDeprovision")
				return
			}
			clusterDeprovision.Status.BlockedResources = blockedResources
			if err := c.Status().Update(context.Background(), clusterDeprovision); err != nil {
				o.logger.WithError(err).Error("could not update ClusterDeprovision")
				return
			}
			return
		},
	)
}

func parseFilter(filterMap aws.Filter, str string) error {
	parts := strings.SplitN(str, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("incorrectly formatted filter")
	}

	filterMap[parts[0]] = parts[1]

	return nil
}
