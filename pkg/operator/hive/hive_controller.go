package hive

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/hive/pkg/resource"

	"github.com/openshift/library-go/pkg/operator/events"

	apiextclientv1beta1 "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1beta1"
	apiregclientv1 "k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset/typed/apiregistration/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// hiveConfigName is the one and only name for a HiveConfig supported in the cluster. Any others will be ignored.
	hiveConfigName = "hive"

	hiveOperatorDeploymentName = "hive-operator"

	managedConfigNamespace    = "openshift-config-managed"
	aggregatorCAConfigMapName = "kube-apiserver-aggregator-client-ca"

	// HiveOperatorNamespaceEnvVar is the environment variable we expect to be given with the namespace the hive-operator is running in.
	HiveOperatorNamespaceEnvVar = "HIVE_OPERATOR_NS"
)

// Add creates a new Hive Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHiveConfig{Client: mgr.GetClient(), scheme: mgr.GetScheme(), restConfig: mgr.GetConfig()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("hive-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	r.(*ReconcileHiveConfig).kubeClient, err = kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHiveConfig).apiregClient, err = apiregclientv1.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHiveConfig).apiextClient, err = apiextclientv1beta1.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHiveConfig).discoveryClient, err = discovery.NewDiscoveryClientForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	r.(*ReconcileHiveConfig).dynamicClient, err = dynamic.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}

	// Regular manager client is not fully initialized here, create our own for some
	// initialization API communication:
	tempClient, err := client.New(mgr.GetConfig(), client.Options{Scheme: mgr.GetScheme()})
	if err != nil {
		return err
	}

	hiveOperatorNS := os.Getenv(HiveOperatorNamespaceEnvVar)
	r.(*ReconcileHiveConfig).hiveOperatorNamespace = hiveOperatorNS
	log.Infof("hive operator NS: %s", hiveOperatorNS)

	// Determine if the openshift-config-managed namespace exists (> v4.0). If so, setup a watch
	// for configmaps in that namespace.
	ns := &corev1.Namespace{}
	log.Debugf("checking for existence of the %s namespace", managedConfigNamespace)
	err = tempClient.Get(context.TODO(), types.NamespacedName{Name: managedConfigNamespace}, ns)
	if err != nil && !errors.IsNotFound(err) {
		log.WithError(err).Errorf("error checking existence of the %s namespace", managedConfigNamespace)
		return err
	}
	if err == nil {
		log.Debugf("the %s namespace exists, setting up a watch for configmaps on it", managedConfigNamespace)
		// Create an informer that only listens to events in the OpenShift managed namespace
		kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(r.(*ReconcileHiveConfig).kubeClient, 10*time.Minute, kubeinformers.WithNamespace(managedConfigNamespace))
		configMapInformer := kubeInformerFactory.Core().V1().ConfigMaps().Informer()
		mgr.Add(&informerRunnable{informer: configMapInformer})

		// Watch for changes to cm/kube-apiserver-aggregator-client-ca in the OpenShift managed namespace
		err = c.Watch(&source.Informer{Informer: configMapInformer}, &handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(aggregatorCAConfigMapHandler)})
		if err != nil {
			return err
		}
		r.(*ReconcileHiveConfig).syncAggregatorCA = true
		r.(*ReconcileHiveConfig).managedConfigCMLister = kubeInformerFactory.Core().V1().ConfigMaps().Lister()
	} else {
		log.Debugf("the %s namespace was not found, skipping watch for the aggregator CA configmap", managedConfigNamespace)
	}

	// Watch for changes to HiveConfig:
	err = c.Watch(&source.Kind{Type: &hivev1.HiveConfig{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Monitor changes to DaemonSets:
	err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Monitor changes to Deployments:
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Monitor changes to Services:
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Lookup the hive-operator Deployment image, we will assume hive components should all be
	// using the same image as the operator.
	operatorDeployment := &appsv1.Deployment{}
	err = tempClient.Get(context.Background(),
		types.NamespacedName{Name: hiveOperatorDeploymentName, Namespace: hiveOperatorNS},
		operatorDeployment)
	if err == nil {
		img := operatorDeployment.Spec.Template.Spec.Containers[0].Image
		pullPolicy := operatorDeployment.Spec.Template.Spec.Containers[0].ImagePullPolicy
		log.Debugf("loaded hive image from hive-operator deployment: %s (%s)", img, pullPolicy)
		r.(*ReconcileHiveConfig).hiveImage = img
		r.(*ReconcileHiveConfig).hiveImagePullPolicy = pullPolicy
	} else {
		log.WithError(err).Fatal("unable to lookup hive image from hive-operator Deployment, image overriding disabled")
	}

	// TODO: Monitor CRDs but do not try to use an owner ref. (as they are global,
	// and our config is namespaced)

	// TODO: it would be nice to monitor the global resources ValidatingWebhookConfiguration
	// and APIService, CRDs, but these cannot have OwnerReferences (which are not namespaced) as they
	// are global. Need to use a different predicate to the Watch function.

	return nil
}

var _ reconcile.Reconciler = &ReconcileHiveConfig{}

// ReconcileHiveConfig reconciles a Hive object
type ReconcileHiveConfig struct {
	client.Client
	scheme                *runtime.Scheme
	kubeClient            kubernetes.Interface
	apiextClient          *apiextclientv1beta1.ApiextensionsV1beta1Client
	apiregClient          *apiregclientv1.ApiregistrationV1Client
	discoveryClient       discovery.DiscoveryInterface
	dynamicClient         dynamic.Interface
	restConfig            *rest.Config
	hiveImage             string
	hiveOperatorNamespace string
	hiveImagePullPolicy   corev1.PullPolicy
	syncAggregatorCA      bool
	managedConfigCMLister corev1listers.ConfigMapLister
}

// Reconcile reads that state of the cluster for a Hive object and makes changes based on the state read
// and what is in the Hive.Spec
func (r *ReconcileHiveConfig) Reconcile(request reconcile.Request) (reconcile.Result, error) {

	hLog := log.WithField("controller", "hive")
	hLog.Info("Reconciling Hive components")

	// Fetch the Hive instance
	instance := &hivev1.HiveConfig{}
	// NOTE: ignoring the Namespace that seems to get set on request when syncing on namespaced objects,
	// when our HiveConfig is ClusterScoped.
	err := r.Get(context.TODO(), types.NamespacedName{Name: request.NamespacedName.Name}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			hLog.Debug("HiveConfig not found, deleted?")
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		hLog.WithError(err).Error("error reading HiveConfig")
		return reconcile.Result{}, err
	}

	// We only support one HiveConfig per cluster, and it must be called "hive". This prevents installing
	// Hive more than once in the cluster.
	if instance.Name != hiveConfigName {
		hLog.WithField("hiveConfig", instance.Name).Warn("invalid HiveConfig name, only one HiveConfig supported per cluster and must be named 'hive'")
		return reconcile.Result{}, nil
	}

	recorder := events.NewRecorder(r.kubeClient.CoreV1().Events(r.hiveOperatorNamespace), "hive-operator", &corev1.ObjectReference{
		Name:      request.Name,
		Namespace: r.hiveOperatorNamespace,
	})

	// Ensure the target namespace for hive components exists and create if not:
	hiveNSName := getHiveNamespace(instance)
	hiveNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: hiveNSName,
		},
	}
	if err := r.Client.Create(context.Background(), hiveNamespace); err != nil {
		if apierrors.IsAlreadyExists(err) {
			hLog.WithField("hiveNS", hiveNSName).Debug("target namespace already exists")
		} else {
			hLog.WithError(err).Error("error creating hive target namespace")
			return reconcile.Result{}, err
		}
	} else {
		hLog.WithField("hiveNS", hiveNSName).Info("target namespace created")
	}

	if r.syncAggregatorCA {
		// We use the configmap lister and not the regular client which only watches resources in the hive namespace
		aggregatorCAConfigMap, err := r.managedConfigCMLister.ConfigMaps(managedConfigNamespace).Get(aggregatorCAConfigMapName)
		// If an error other than not found, retry. If not found, it means we don't need to do anything with
		// admission pods yet.
		cmLog := hLog.WithField("configmap", fmt.Sprintf("%s/%s", managedConfigNamespace, aggregatorCAConfigMapName))
		switch {
		case errors.IsNotFound(err):
			cmLog.Warningf("configmap was not found, will not sync aggregator CA with admission pods")
		case err != nil:
			cmLog.WithError(err).Errorf("cannot retrieve configmap")
			return reconcile.Result{}, err
		default:
			caHash := computeHash(aggregatorCAConfigMap.Data)
			cmLog.WithField("hash", caHash).Debugf("computed hash for configmap")
			if instance.Status.AggregatorClientCAHash != caHash {
				cmLog.WithField("oldHash", instance.Status.AggregatorClientCAHash).
					Info("configmap has changed, admission pods will restart on the next sync")
				instance.Status.AggregatorClientCAHash = caHash
				cmLog.Debugf("updating status with new aggregator CA configmap hash")
				err := r.updateHiveConfigStatus(instance, cmLog, true)
				if err != nil {
					cmLog.WithError(err).Error("cannot update hash in config status")
				}
				return reconcile.Result{}, err
			}
			cmLog.Debug("configmap unchanged, nothing to do")
		}
	}

	h := resource.NewHelperFromRESTConfig(r.restConfig, hLog)

	managedDomainsConfigMap, err := r.configureManagedDomains(hLog, instance)
	if err != nil {
		hLog.WithError(err).Error("error setting up managed domains")
		r.updateHiveConfigStatus(instance, hLog, false)
		return reconcile.Result{}, err
	}

	err = r.deployHive(hLog, h, instance, recorder, managedDomainsConfigMap)
	if err != nil {
		hLog.WithError(err).Error("error deploying Hive")
		r.updateHiveConfigStatus(instance, hLog, false)
		return reconcile.Result{}, err
	}

	// Cleanup legacy objects:
	if err := r.cleanupLegacyObjects(hLog); err != nil {
		return reconcile.Result{}, err
	}

	err = r.deployHiveAdmission(hLog, h, instance, recorder, managedDomainsConfigMap)
	if err != nil {
		hLog.WithError(err).Error("error deploying HiveAdmission")
		r.updateHiveConfigStatus(instance, hLog, false)
		return reconcile.Result{}, err
	}

	err = r.tearDownLegacyHiveAPI(hLog, hiveNSName)
	if err != nil {
		hLog.WithError(err).Error("error tearing down Hive v1alpha1 aggregated API")
		r.updateHiveConfigStatus(instance, hLog, false)
		return reconcile.Result{}, err
	}

	r.updateHiveConfigStatus(instance, hLog, true)
	return reconcile.Result{}, nil
}

func (r *ReconcileHiveConfig) cleanupLegacyObjects(hLog log.FieldLogger) error {
	clusterRoleGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "clusterroles",
	}
	clusterRoleBindingGVR := schema.GroupVersionResource{
		Group:    "rbac.authorization.k8s.io",
		Version:  "v1",
		Resource: "clusterrolebindings",
	}
	if err := dynamicDelete(r.dynamicClient, clusterRoleGVR, "manager-role", hLog); err != nil {
		return err
	}
	if err := dynamicDelete(r.dynamicClient, clusterRoleBindingGVR, "manager-rolebinding", hLog); err != nil {
		return err
	}
	return nil
}

type informerRunnable struct {
	informer cache.SharedIndexInformer
}

func (r *informerRunnable) Start(stopch <-chan struct{}) error {
	r.informer.Run(stopch)
	cache.WaitForCacheSync(stopch, r.informer.HasSynced)
	return nil
}

func aggregatorCAConfigMapHandler(o handler.MapObject) []reconcile.Request {
	if o.Meta.GetName() == aggregatorCAConfigMapName {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: hiveConfigName}}}
	}
	return nil
}

func computeHash(data map[string]string) string {
	hasher := md5.New()
	hasher.Write([]byte(fmt.Sprintf("%v", data)))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (r *ReconcileHiveConfig) updateHiveConfigStatus(hc *hivev1.HiveConfig, logger log.FieldLogger, succeeded bool) error {
	hc.Status.ObservedGeneration = hc.Generation
	hc.Status.ConfigApplied = succeeded

	err := r.Status().Update(context.TODO(), hc)
	if err != nil {
		logger.WithError(err).Error("failed to update HiveConfig status")
	}
	return err
}
