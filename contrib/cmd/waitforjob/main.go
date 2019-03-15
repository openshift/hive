package main

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	defaultJobExistenceTimeout = 5 * time.Minute
	defaultJobExecutionTimeout = 90 * time.Minute
)

const (
	defaultLogLevel = "info"
)

func main() {
	opts := &waitForJobOpts{}

	cmd := &cobra.Command{
		Use:   "waitforjob JOBNAME [OPTIONS]",
		Short: "wait for job",
		Long:  "Contains various utilities for running and testing hive",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				cmd.Usage()
				os.Exit(1)
			}
			opts.jobName = args[0]
			opts.Run()
		},
	}
	cmd.PersistentFlags().StringVar(&opts.logLevel, "log-level", defaultLogLevel, "Log level (debug,info,warn,error,fatal)")
	cmd.PersistentFlags().DurationVar(&opts.existenceTimeout, "job-existence-timeout", defaultJobExistenceTimeout, "Maximum time to wait for the named job to be created")
	cmd.PersistentFlags().DurationVar(&opts.executionTimeout, "job-execution-timeout", defaultJobExecutionTimeout, "Maximum time to wait for the job to execute")
	cmd.PersistentFlags().BoolVar(&opts.notFoundOK, "not-found-ok", false, "Do not exit with an error if the job is not found")
	cmd.Execute()
}

type waitForJobOpts struct {
	logLevel         string
	jobName          string
	notFoundOK       bool
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
		log.WithError(err).Fatal("failed to obtain client")
	}

	if err := w.waitForJobExistence(client, namespace); err != nil {
		log.WithError(err).Fatal("job existence failed")
	}

	if err := w.waitForJobExecution(client, namespace); err != nil {
		log.WithError(err).Fatal("job execution failed")
	}
}

func (w *waitForJobOpts) waitForJobExistence(client clientset.Interface, namespace string) error {
	logger := log.WithField("waiting-for-existence", fmt.Sprintf("job (%s/%s)", namespace, w.jobName))
	err := wait.PollImmediate(10*time.Second, w.existenceTimeout, func() (bool, error) {
		logger.Debug("Retrieving job")
		_, err := client.BatchV1().Jobs(namespace).Get(w.jobName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Debug("job does not exist yet")
			} else {
				logger.WithError(err).Warning("unexpected error retrieving job")
			}
			return false, nil
		}
		logger.Info("Job found")
		return true, nil
	})
	if err == wait.ErrWaitTimeout && w.notFoundOK {
		logger.Warn("timed out waiting for job to exist, it may have already been deleted")
		return nil
	}
	return err
}

func (w *waitForJobOpts) waitForJobExecution(client clientset.Interface, namespace string) error {
	logger := log.WithField("waiting-for-run", fmt.Sprintf("job (%s/%s)", namespace, w.jobName))
	err := wait.PollImmediate(30*time.Second, w.executionTimeout, func() (bool, error) {
		logger.Debug("Retrieving job")
		job, err := client.BatchV1().Jobs(namespace).Get(w.jobName, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) && w.notFoundOK {
				logger.Warn("Job no longer exists. It may have already been deleted")
				return true, nil
			}
			logger.WithError(err).Error("Could not fetch job")
			return false, err
		}
		if isFailed(job) {
			logger.Error("Job has failed")
			return false, fmt.Errorf("job %s/%s has failed", namespace, w.jobName)
		}
		if isSuccessful(job) {
			logger.Info("Job has finished successfully")
			return true, nil
		}
		logger.Debug("Job has not completed yet")
		return false, nil
	})
	return err
}

func (w *waitForJobOpts) localClient() (clientset.Interface, string, error) {
	log.Debug("Creating cluster client")
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

	kubeClient, err := clientset.NewForConfig(cfg)
	if err != nil {
		log.WithError(err).Error("Cannot create kubernetes client from client config")
		return nil, "", err
	}

	return kubeClient, namespace, nil
}

func getJobConditionStatus(job *batchv1.Job, conditionType batchv1.JobConditionType) corev1.ConditionStatus {
	for _, condition := range job.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status
		}
	}
	return corev1.ConditionFalse
}

func isSuccessful(job *batchv1.Job) bool {
	return getJobConditionStatus(job, batchv1.JobComplete) == corev1.ConditionTrue
}

func isFailed(job *batchv1.Job) bool {
	return getJobConditionStatus(job, batchv1.JobFailed) == corev1.ConditionTrue
}
