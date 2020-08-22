package destroycluster

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	batchv1client "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/tools/cache"
	clientwatch "k8s.io/client-go/tools/watch"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hiveclient "github.com/openshift/hive/pkg/client/clientset/versioned"
	"github.com/openshift/hive/pkg/constants"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
	"github.com/openshift/hive/test/e2e/common"
)

const (
	timeout = 30 * time.Minute
)

func TestDestroyCluster(t *testing.T) {
	logger := log.WithField("test", "TestDestroyCluster")
	logger.Info("Starting destroy cluster test")

	cd := common.MustGetClusterDeployment()
	logger = logger.WithField("cluster", fmt.Sprintf("%s/%s", cd.Namespace, cd.Name))
	if !cd.Spec.Installed {
		logger.Warn("Cluster deployment did not finish installing")
	}

	fail := failTestFunc(t, logger)

	c := common.MustGetHiveClient()
	logger.Info("Deleting cluster deployment")
	err := c.HiveV1().ClusterDeployments(cd.Namespace).Delete(context.TODO(), cd.Name, metav1.DeleteOptions{})
	if err != nil {
		fail("Failed to delete cluster deployment: %v", err)
	}

	var jobObserved int32 = 0
	go monitorDeprovisionJob(cd, &jobObserved, logger)

	logger.Info("Waiting for cluster deployment to go away")
	err = waitForClusterDeploymentToGoAway(cd, c)
	if err != nil {
		fail("Failed to wait for cluster deployment to go away: %v", err)
	}
	if atomic.LoadInt32(&jobObserved) == 0 && cd.Spec.ClusterMetadata != nil {
		fail("Expected to see a deprovision job, but none was observed")
	}
	logger.Info("Cluster deployment has been removed")

	// Ensure that no dnszones owned by the clusterdeployment are left over
	zoneLabelSelector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: map[string]string{
			constants.DNSZoneTypeLabel:           constants.DNSZoneTypeChild,
			constants.ClusterDeploymentNameLabel: cd.Name,
		},
	})
	if err != nil {
		fail("Failed to create a dns zone label selector: %v", err)
	}
	dnsZoneList, err := c.HiveV1().DNSZones(cd.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: zoneLabelSelector.String(),
	})
	if err != nil {
		fail("Failed to get dns zone list: %v", err)
	}
	if len(dnsZoneList.Items) > 0 {
		fail("At least one DNSZone still exists after clusterdeployment deletion")
	}
	logger.Info("No DNS Zones related to the cluster exist")
}

// monitorDeprovisionJob will look for a deprovision job and save its log
func monitorDeprovisionJob(cd *hivev1.ClusterDeployment, jobObserved *int32, logger *log.Entry) {
	if cd.Spec.ClusterMetadata == nil {
		logger.Warn("Cluster metadata was never set, not monitoring uninstall job")
		return
	}

	cfg := common.MustGetConfig()
	batchClient, err := batchv1client.NewForConfig(cfg)
	if err != nil {
		logger.WithError(err).Error("Cannot create batch client")
		return
	}
	deprovisionJobName := install.GetUninstallJobName(cd.Name)
	logger = logger.WithField("job", deprovisionJobName)
	nameFilter := func(options *metav1.ListOptions) {
		options.FieldSelector = fmt.Sprintf("metadata.name=%s", deprovisionJobName)
	}
	listWatcher := cache.NewFilteredListWatchFromClient(batchClient.RESTClient(), "jobs", cd.Namespace, nameFilter)
	var writeLogOnce sync.Once
	monitor := func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Added:
			job := event.Object.(*batchv1.Job)
			atomic.AddInt32(jobObserved, 1)
			logger.Info("Deprovision job created")
			go writeLogOnce.Do(func() { writeJobLog(job, logger) })
		case watch.Modified:
			job := event.Object.(*batchv1.Job)
			atomic.AddInt32(jobObserved, 1)
			if controllerutils.IsFinished(job) {
				logger.Info("Deprovision job has finished")
				return true, nil
			}
		}
		return false, nil
	}
	_, err = clientwatch.UntilWithSync(context.TODO(), listWatcher, &batchv1.Job{}, nil, monitor)
	if err != nil {
		logger.WithError(err).Error("Cannot watch client")
		return
	}
}

func writeJobLog(job *batchv1.Job, logger *log.Entry) {
	if os.Getenv("ARTIFACT_DIR") == "" {
		logger.Warn("ARTIFACT_DIR is not set, not writing uninstall logs")
		return
	}
	client := common.MustGetKubernetesClient()
	podLabelSelector, err := metav1.LabelSelectorAsSelector(job.Spec.Selector)
	if err != nil {
		logger.WithError(err).Error("Could not create pod selector from job")
		return
	}
	podFilter := func(options *metav1.ListOptions) {
		options.LabelSelector = podLabelSelector.String()
	}
	var pod *corev1.Pod
	waitForPodRunning := func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Added, watch.Modified:
			pod = event.Object.(*corev1.Pod)
			if pod.Status.Phase == corev1.PodRunning {
				return true, nil
			}
		}
		return false, nil
	}
	listWatcher := cache.NewFilteredListWatchFromClient(client.CoreV1().RESTClient(), "pods", job.Namespace, podFilter)
	_, err = clientwatch.UntilWithSync(context.TODO(), listWatcher, &corev1.Pod{}, nil, waitForPodRunning)
	if err != nil {
		logger.WithError(err).Error("failed to watch client")
		return
	}
	request := client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Follow: true})
	readCloser, err := request.Stream(context.TODO())
	if err != nil {
		logger.WithError(err).Error("failed to stream job logs")
		return
	}
	defer readCloser.Close()
	out, err := os.Create(filepath.Join(os.Getenv("ARTIFACT_DIR"), "hive_uninstall_job.log"))
	if err != nil {
		logger.WithError(err).Error("failed to create log file")
		return
	}
	defer out.Close()
	logger.Infof("Saving logs to %s", out.Name())
	r := bufio.NewReader(readCloser)
	for {
		bytes, err := r.ReadBytes('\n')
		if _, err := out.Write(bytes); err != nil {
			logger.WithError(err).Error("Cannot write out to log file")
			return
		}
		if err == io.EOF {
			return
		}
		if err != nil {
			logger.WithError(err).Error("Cannot read from job pod log stream")
			return
		}
	}
}

func waitForClusterDeploymentToGoAway(cd *hivev1.ClusterDeployment, client hiveclient.Interface) error {
	waitContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	nameFilter := func(options *metav1.ListOptions) {
		options.FieldSelector = fmt.Sprintf("metadata.name=%s", cd.Name)
	}
	listWatcher := cache.NewFilteredListWatchFromClient(client.HiveV1().RESTClient(), "clusterdeployments", cd.Namespace, nameFilter)
	checkIfExists := func(store cache.Store) (bool, error) {
		return len(store.List()) == 0, nil
	}
	isDeleted := func(event watch.Event) (bool, error) {
		return event.Type == watch.Deleted, nil
	}
	_, err := clientwatch.UntilWithSync(waitContext, listWatcher, &hivev1.ClusterDeployment{}, checkIfExists, isDeleted)
	if err != nil {
		return errors.Wrap(err, "failed to wait for cluster deployment to be deleted")
	}
	return nil
}

func failTestFunc(t *testing.T, logger *log.Entry) func(string, ...interface{}) {
	return func(format string, args ...interface{}) {
		log.Error(fmt.Sprintf(format, args...))
		t.Fatalf(format, args...)
	}
}
