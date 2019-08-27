package main

import (
	"context"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	clientwatch "k8s.io/client-go/tools/watch"

	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	defaultLogLevel            = "info"
	defaultJobExistenceTimeout = 5 * time.Minute
	defaultJobExecutionTimeout = 90 * time.Minute
	installJobType             = "install"
	uninstallJobType           = "uninstall"
)

func main() {
	opts := &waitForJobOpts{}

	cmd := &cobra.Command{
		Use:   "waitforjob CLUSTERNAME JOBTYPE [OPTIONS]",
		Short: "wait for job",
		Long:  "Contains various utilities for running and testing hive",
		Args: func(cmd *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(2)(cmd, args); err != nil {
				return err
			}
			switch args[1] {
			case installJobType, uninstallJobType:
			default:
				return fmt.Errorf("invalid job type %s: must be one of %v", opts.jobType, []string{installJobType, uninstallJobType})
			}
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {
			opts.clusterName = args[0]
			opts.jobType = args[1]
			opts.Run()
		},
	}
	cmd.PersistentFlags().StringVar(&opts.logLevel, "log-level", defaultLogLevel, "Log level (debug,info,warn,error,fatal)")
	cmd.PersistentFlags().DurationVar(&opts.existenceTimeout, "job-existence-timeout", defaultJobExistenceTimeout, "Maximum time to wait for the named job to be created")
	cmd.PersistentFlags().DurationVar(&opts.executionTimeout, "job-execution-timeout", defaultJobExecutionTimeout, "Maximum time to wait for the job to execute")
	cmd.Execute()
}

type waitForJobOpts struct {
	logLevel         string
	clusterName      string
	jobType          string
	existenceTimeout time.Duration
	executionTimeout time.Duration
}

func (w *waitForJobOpts) Run() {
	// Set log level
	level, err := log.ParseLevel(w.logLevel)
	if err != nil {
		log.WithError(err).Fatal("Cannot parse log level")
	}
	log.SetLevel(level)
	log.Debug("debug logging enabled")

	client, namespace, err := w.localClient()
	if err != nil {
		log.WithError(err).Fatal("Could not create a local client")
	}

	if w.jobType == uninstallJobType {
		switch jobFound, err := w.waitForJobExecution(client, namespace, w.existenceTimeout); {
		case err == nil:
			return
		case err != wait.ErrWaitTimeout:
			log.WithError(err).Fatal("job existence failed")
		case !jobFound:
			log.Warn("timed out waiting for job to exist, it may have already been deleted")
			return
		}
	}

	if _, err := w.waitForJobExecution(client, namespace, w.executionTimeout); err != nil {
		log.WithError(err).Fatal("job execution failed")
	}
}

func (w *waitForJobOpts) waitForJobExecution(client *kubeclient.Clientset, namespace string, timeout time.Duration) (bool, error) {
	logger := log.WithField("waiting-for-run", w.jobType).WithField("namespace", namespace).WithField("cluster", w.clusterName)

	labelSet := labels.Set{}
	labelSet[constants.ClusterDeploymentNameLabel] = w.clusterName
	switch w.jobType {
	case installJobType:
		labelSet[constants.InstallJobLabel] = "true"
	case uninstallJobType:
		labelSet[constants.UninstallJobLabel] = "true"
	}

	waitContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	jobFound := false
	_, err := clientwatch.UntilWithSync(
		waitContext,
		cache.NewFilteredListWatchFromClient(
			client.BatchV1().RESTClient(),
			"jobs",
			namespace,
			func(options *metav1.ListOptions) {
				options.LabelSelector = labelSet.String()
			}),
		&batchv1.Job{},
		nil,
		func(event watch.Event) (bool, error) {
			switch event.Type {
			case watch.Added, watch.Modified:
				job, ok := event.Object.(*batchv1.Job)
				if !ok {
					logger.Warnf("Expected a Job object but got a %q object instead", event.Object.GetObjectKind().GroupVersionKind())
					return false, nil
				}
				if !jobFound {
					logger.WithField("job", job.Name).Info("Job found")
					jobFound = true
				}
				switch {
				case controllerutils.IsFailed(job):
					logger.Error("Job has failed")
					return false, fmt.Errorf("job %s/%s has failed", job.Namespace, job.Name)
				case controllerutils.IsSuccessful(job):
					logger.Info("Job has finished successfully")
					return true, nil
				}
			}
			logger.Debug("Job has not completed yet")
			return false, nil
		},
	)
	return jobFound, err
}

func (w *waitForJobOpts) localClient() (*kubeclient.Clientset, string, error) {
	log.Debug("Creating cluster client config")
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	cfg, err := kubeconfig.ClientConfig()
	if err != nil {
		log.WithError(err).Error("Cannot obtain client config")
		return nil, "", err
	}
	namespace, _, err := kubeconfig.Namespace()
	if err != nil {
		log.WithError(err).Error("Cannot obtain default namespace from client config")
		return nil, "", err
	}
	client, err := kubeclient.NewForConfig(cfg)
	if err != nil {
		log.WithError(err).Error("failed to create a config client")
		return nil, "", err
	}
	return client, namespace, nil
}
