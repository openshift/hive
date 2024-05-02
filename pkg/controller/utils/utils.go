package utils

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"time"

	"golang.org/x/time/rate"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apihelpers "github.com/openshift/hive/apis/helpers"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
)

const (
	// defaultQueueQPS is the default workqueue qps, same as used in DefaultControllerRateLimiter
	defaultQueueQPS = 10
	// defaultQueueBurst is the default workqueue burst, same as used in DefaultControllerRateLimiter
	defaultQueueBurst = 100
	// defaultConcurrentReconciles is the default number of concurrent reconciles
	defaultConcurrentReconciles = 5
	// ConcurrentReconcilesEnvVariableFormat is the format of the environment variable
	// that stores concurrent reconciles for a controller
	ConcurrentReconcilesEnvVariableFormat = "%s-concurrent-reconciles"

	// ClientQPSEnvVariableFormat is the format of the environment variable that stores
	// client QPS for a controller
	ClientQPSEnvVariableFormat = "%s-client-qps"

	// ClientBurstEnvVariableFormat is the format of the environment variable that stores
	// client burst for a controller
	ClientBurstEnvVariableFormat = "%s-client-burst"

	// QueueQPSEnvVariableFormat is the format of the environment variable that stores
	// workqueue QPS for a controller
	QueueQPSEnvVariableFormat = "%s-queue-qps"

	// QueueBurstEnvVariableFormat is the format of the environment variable that stores
	// workqueue burst for a controller
	QueueBurstEnvVariableFormat = "%s-queue-burst"
)

// HasFinalizer returns true if the given object has the given finalizer
func HasFinalizer(object metav1.Object, finalizer string) bool {
	for _, f := range object.GetFinalizers() {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds a finalizer to the given object
func AddFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Insert(finalizer)
	object.SetFinalizers(finalizers.List())
}

// DeleteFinalizer removes a finalizer from the given object
func DeleteFinalizer(object metav1.Object, finalizer string) {
	finalizers := sets.NewString(object.GetFinalizers()...)
	finalizers.Delete(finalizer)
	object.SetFinalizers(finalizers.List())
}

// getConcurrentReconciles returns the number of goroutines each controller should
// use for parallel processing of their queue. Default value, if not set in
// hive-controllers-config, will be 5.
func getConcurrentReconciles(controllerName hivev1.ControllerName) (int, error) {
	if value, ok := getValueFromEnvVariable(controllerName, ConcurrentReconcilesEnvVariableFormat); ok {
		concurrentReconciles, err := strconv.Atoi(value)
		if err != nil {
			return 0, err
		}
		return concurrentReconciles, nil
	}
	return defaultConcurrentReconciles, nil
}

// getClientRateLimiter returns the client rate limiter for the controller
func getClientRateLimiter(controllerName hivev1.ControllerName) (flowcontrol.RateLimiter, error) {
	qps := rest.DefaultQPS
	if value, ok := getValueFromEnvVariable(controllerName, ClientQPSEnvVariableFormat); ok {
		qpsInt, err := strconv.Atoi(value)
		if err != nil {
			return nil, err
		}
		qps = float32(qpsInt)
	}

	burst := rest.DefaultBurst
	if value, ok := getValueFromEnvVariable(controllerName, ClientBurstEnvVariableFormat); ok {
		var err error
		burst, err = strconv.Atoi(value)
		if err != nil {
			return nil, err
		}
	}

	return flowcontrol.NewTokenBucketRateLimiter(qps, burst), nil
}

// getQueueRateLimiter returns the workqueue rate limiter for the controller
func getQueueRateLimiter(controllerName hivev1.ControllerName) (workqueue.RateLimiter, error) {
	var err error
	qps := defaultQueueQPS
	if value, ok := getValueFromEnvVariable(controllerName, QueueQPSEnvVariableFormat); ok {
		qps, err = strconv.Atoi(value)
		if err != nil {
			return nil, err
		}
	}

	burst := defaultQueueBurst
	if value, ok := getValueFromEnvVariable(controllerName, QueueBurstEnvVariableFormat); ok {
		burst, err = strconv.Atoi(value)
		if err != nil {
			return nil, err
		}
	}

	return workqueue.NewMaxOfRateLimiter(
		workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 1000*time.Second),
		&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(qps), burst)},
	), nil
}

func GetControllerConfig(client client.Client, controllerName hivev1.ControllerName) (int, flowcontrol.RateLimiter, workqueue.RateLimiter, error) {
	concurrentReconciles, err := getConcurrentReconciles(controllerName)
	if err != nil {
		return 0, nil, nil, err
	}
	clientRateLimiter, err := getClientRateLimiter(controllerName)
	if err != nil {
		return 0, nil, nil, err
	}
	queueRateLimiter, err := getQueueRateLimiter(controllerName)
	if err != nil {
		return 0, nil, nil, err
	}
	return concurrentReconciles, clientRateLimiter, queueRateLimiter, nil
}

// MergeJsons will merge the global and local pull secret and return it
func MergeJsons(globalPullSecret string, localPullSecret string, cdLog log.FieldLogger) (string, error) {

	type dockerConfig map[string]interface{}
	type dockerConfigJSON struct {
		Auths dockerConfig `json:"auths"`
	}

	var mGlobal, mLocal dockerConfigJSON
	jGlobal := []byte(globalPullSecret)
	err := json.Unmarshal(jGlobal, &mGlobal)
	if err != nil {
		return "", err
	}

	jLocal := []byte(localPullSecret)
	err = json.Unmarshal(jLocal, &mLocal)
	if err != nil {
		return "", err
	}

	for k, v := range mLocal.Auths {
		if _, ok := mGlobal.Auths[k]; ok {
			cdLog.Infof("The auth for %s from cluster deployment pull secret is used instead of global pull secret", k)
		}
		mGlobal.Auths[k] = v
	}
	jMerged, err := json.Marshal(mGlobal)
	if err != nil {
		return "", err
	}
	return string(jMerged), nil
}

// GetChecksumOfObject returns the md5sum hash of the object passed in.
func GetChecksumOfObject(object interface{}) (string, error) {
	b, err := json.Marshal(object)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", md5.Sum(b)), nil
}

// GetChecksumOfObjects returns the md5sum hash of the objects passed in.
func GetChecksumOfObjects(objects ...interface{}) (string, error) {
	return GetChecksumOfObject(objects)
}

// DNSZoneName returns the predictable name for a DNSZone for the given ClusterDeployment.
func DNSZoneName(cdName string) string {
	return apihelpers.GetResourceName(cdName, "zone")
}

// LogLevel returns the log level to use to log the specified error.
func LogLevel(err error) log.Level {
	if err == nil {
		return log.ErrorLevel
	}
	for {
		switch {
		case apierrors.IsAlreadyExists(err),
			apierrors.IsConflict(err),
			apierrors.IsNotFound(err):
			return log.InfoLevel
		}
		// It would be easier to use errors.Cause(), but unfortunately with that there is no way to safely tell when
		// the error does not have a cause. We used to check that the cause returned from errors.Cause() was equal to
		// the original error. However, that causes a runtime panic if the error is a non-comparable type.
		type causer interface {
			Cause() error
		}
		if cause, ok := err.(causer); !ok {
			return log.ErrorLevel
		} else {
			err = cause.Cause()
		}
	}
}

// ListRuntimeObjects returns a slice of runtime objects returned from the kubernetes client based on the passed in list of types to return and list options.
func ListRuntimeObjects(c client.Client, typesToList []client.ObjectList, opts ...client.ListOption) ([]runtime.Object, error) {
	nsObjects := []runtime.Object{}

	for _, t := range typesToList {
		listObj := t.DeepCopyObject().(client.ObjectList)
		if err := c.List(context.TODO(), listObj, opts...); err != nil {
			return nil, err
		}
		list, err := meta.ExtractList(listObj)
		if err != nil {
			return nil, err
		}

		nsObjects = append(nsObjects, list...)
	}

	return nsObjects, nil
}

// GetHiveNamespace determines the namespace where core hive components run (hive-controllers, hiveadmission), by checking
// for the required environment variable.
func GetHiveNamespace() string {
	envNamespace := os.Getenv(constants.HiveNamespaceEnvVar)
	if envNamespace != "" {
		return envNamespace
	}
	// Returning a default here, mostly for unit test simplicity and to avoid having to pass this around to all controllers and libraries..
	return constants.DefaultHiveNamespace
}

// getValueFromEnvVariable gets a configuration value for a controller from the environment variable
func getValueFromEnvVariable(controllerName hivev1.ControllerName, envVarFormat string) (string, bool) {
	if value, ok := os.LookupEnv(fmt.Sprintf(envVarFormat, controllerName)); ok {
		return value, true
	}
	if value, ok := os.LookupEnv(fmt.Sprintf(envVarFormat, "default")); ok {
		return value, true
	}
	return "", false
}

// EnsureRequeueAtLeastWithin ensures that the requeue of the object will occur within the given duration. If the
// reconcile result and error will already result in a requeue soon enough, then the supplied reconcile result and error
// will be returned as is.
func EnsureRequeueAtLeastWithin(duration time.Duration, result reconcile.Result, err error) (reconcile.Result, error) {
	if err != nil {
		return result, err
	}
	if result.Requeue && result.RequeueAfter <= 0 {
		return result, err
	}
	if ra := result.RequeueAfter; 0 < ra && ra < duration {
		return result, err
	}
	return reconcile.Result{RequeueAfter: duration, Requeue: true}, nil
}

// CopySecret copies the secret defined by src to dest.
func CopySecret(c client.Client, src, dest types.NamespacedName, owner metav1.Object, scheme *runtime.Scheme) error {
	srcSecret := &corev1.Secret{}
	if err := c.Get(context.Background(), src, srcSecret); err != nil {
		return err
	}

	destSecret := &corev1.Secret{}
	err := c.Get(context.Background(), dest, destSecret)
	if apierrors.IsNotFound(err) {
		destSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dest.Name,
				Namespace: dest.Namespace,
			},
			Data:       srcSecret.DeepCopy().Data,
			StringData: srcSecret.DeepCopy().StringData,
			Type:       srcSecret.Type,
		}
		if owner != nil {
			if err := controllerutil.SetOwnerReference(owner, destSecret, scheme); err != nil {
				return nil
			}
		}

		return c.Create(context.Background(), destSecret)
	}
	if err != nil {
		return err
	}

	if reflect.DeepEqual(destSecret.Data, srcSecret.Data) && reflect.DeepEqual(destSecret.StringData, srcSecret.StringData) {
		return nil // no work as the dest and source data matches.
	}

	destSecret.Data = srcSecret.DeepCopy().Data
	destSecret.StringData = srcSecret.DeepCopy().StringData
	return c.Update(context.Background(), destSecret)
}

// BuildControllerLogger returns a logger for controllers with consistent fields.
func BuildControllerLogger(controller hivev1.ControllerName, resource string, nsName types.NamespacedName) *log.Entry {
	return log.WithFields(log.Fields{
		"controller":  controller,
		resource:      nsName.String(),
		"reconcileID": utilrand.String(constants.ReconcileIDLen),
	})
}

// SetProxyEnvVars will add the standard proxy environment variables to all containers in the given pod spec. If
// any of the provided values are empty, the environment variable will not be set.
func SetProxyEnvVars(podSpec *corev1.PodSpec, httpProxy, httpsProxy, noProxy string) {
	setEnvVarOnContainers := func(podSpec *corev1.PodSpec, envVar, value string) {
		if value == "" {
			return
		}
		for i := range podSpec.Containers {
			// Check if the env var is already set, if so we preserve the original but warn if they differ:
			found := false
			for j := range podSpec.Containers[i].Env {
				if podSpec.Containers[i].Env[j].Name == envVar {
					found = true
					if podSpec.Containers[i].Env[j].Value != value {
						log.Warnf("container %s already has env var %s=%s, overwriting with %s", podSpec.Containers[i].Name, envVar, podSpec.Containers[i].Env[j].Value, value)
						podSpec.Containers[i].Env[j].Value = value
					}
				}
			}
			if !found {
				log.WithField(envVar, value).Info("transferring env var to PodSpec")
				podSpec.Containers[i].Env = append(podSpec.Containers[i].Env, corev1.EnvVar{Name: envVar, Value: value})
			}
		}
	}

	if httpProxy != "" {
		setEnvVarOnContainers(podSpec, "HTTP_PROXY", httpProxy)
	}
	if httpsProxy != "" {
		setEnvVarOnContainers(podSpec, "HTTPS_PROXY", httpsProxy)
	}
	if noProxy != "" {
		setEnvVarOnContainers(podSpec, "NO_PROXY", noProxy)
	}
}

func SafeDelete(cl client.Client, ctx context.Context, obj client.Object) error {
	rv := obj.GetResourceVersion()
	return cl.Delete(ctx, obj, client.Preconditions{ResourceVersion: &rv})
}

func GetThisPod(cl client.Client) (*corev1.Pod, error) {
	podName := os.Getenv("POD_NAME")
	podNamespace := os.Getenv("POD_NAMESPACE")
	if podName == "" || podNamespace == "" {
		return nil, fmt.Errorf("missing POD_NAME (%q) and/or POD_NAMESPACE (%q) env", podName, podNamespace)
	}
	po := &corev1.Pod{}
	if err := cl.Get(context.TODO(), types.NamespacedName{Namespace: podNamespace, Name: podName}, po); err != nil {
		return nil, err
	}
	return po, nil
}
