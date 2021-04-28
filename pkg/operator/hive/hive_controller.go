package hive

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/resource"

	"github.com/openshift/library-go/pkg/operator/events"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
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

	// watchResyncInterval is used for a couple handcrafted watches we do with our own informers.
	watchResyncInterval = 30 * time.Minute
)

// Add creates a new Hive Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHiveConfig{
		Client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		restConfig: mgr.GetConfig(),
		mgr:        mgr,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("hive-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Provide a ref to the controller on the reconciler, which is used to establish a watch on
	// secrets in the hive namespace, which isn't known until we have a HiveConfig.
	r.(*ReconcileHiveConfig).ctrlr = c

	r.(*ReconcileHiveConfig).kubeClient, err = kubernetes.NewForConfig(mgr.GetConfig())
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
		kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(r.(*ReconcileHiveConfig).kubeClient, watchResyncInterval, kubeinformers.WithNamespace(managedConfigNamespace))
		configMapInformer := kubeInformerFactory.Core().V1().ConfigMaps().Informer()
		mgr.Add(&informerRunnable{informer: configMapInformer})

		// Watch for changes to cm/kube-apiserver-aggregator-client-ca in the OpenShift managed namespace
		err = c.Watch(&source.Informer{Informer: configMapInformer}, handler.EnqueueRequestsFromMapFunc(handler.MapFunc(aggregatorCAConfigMapHandler)))
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

	err = mgr.GetFieldIndexer().IndexField(context.TODO(), &hivev1.HiveConfig{}, "spec.secrets.secretName", func(o client.Object) []string {
		var res []string
		instance := o.(*hivev1.HiveConfig)

		// add all the secret objects to res that should trigger resync of HiveConfig

		for _, lObj := range instance.Spec.AdditionalCertificateAuthoritiesSecretRef {
			res = append(res, lObj.Name)
		}

		return res
	})
	if err != nil {
		return err
	}

	// Monitor changes to DaemonSets:
	err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		OwnerType: &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Monitor changes to Deployments:
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		OwnerType: &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Monitor changes to Services:
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		OwnerType: &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Monitor changes to StatefulSets:
	err = c.Watch(&source.Kind{Type: &appsv1.StatefulSet{}}, &handler.EnqueueRequestForOwner{
		OwnerType: &hivev1.HiveConfig{},
	})
	if err != nil {
		return err
	}

	// Monitor CRDs so that we can keep latest list of supported contracts
	err = c.Watch(&source.Kind{Type: &apiextv1beta1.CustomResourceDefinition{}},
		handler.EnqueueRequestsFromMapFunc(func(_ client.Object) []reconcile.Request {
			retval := []reconcile.Request{}

			configList := &hivev1.HiveConfigList{}
			err := r.(*ReconcileHiveConfig).List(context.TODO(), configList) // reconcile all HiveConfigs
			if err != nil {
				log.WithError(err).Errorf("error listing hive configs for CRD reconcile")
				return retval
			}

			for _, config := range configList.Items {
				retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
					Name: config.Name,
				}})
			}
			log.WithField("configs", retval).Debug("reconciled for change in CRD")
			return retval
		}))
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
	scheme                 *runtime.Scheme
	kubeClient             kubernetes.Interface
	discoveryClient        discovery.DiscoveryInterface
	dynamicClient          dynamic.Interface
	restConfig             *rest.Config
	hiveImage              string
	hiveOperatorNamespace  string
	hiveImagePullPolicy    corev1.PullPolicy
	syncAggregatorCA       bool
	managedConfigCMLister  corev1listers.ConfigMapLister
	ctrlr                  controller.Controller
	hiveSecretLister       corev1listers.SecretLister
	secretWatchEstablished bool
	mgr                    manager.Manager
}

// Reconcile reads that state of the cluster for a Hive object and makes changes based on the state read
// and what is in the Hive.Spec
func (r *ReconcileHiveConfig) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	hLog := log.WithField("controller", "hive")
	hLog.Info("Reconciling Hive components")

	// Fetch the Hive instance
	instance := &hivev1.HiveConfig{}

	// We only support one HiveConfig per cluster, and it must be called "hive". This prevents installing
	// Hive more than once in the cluster.
	if request.NamespacedName.Name != hiveConfigName {
		hLog.WithField("hiveConfig", request.NamespacedName.Name).Warn(
			"invalid HiveConfig name, only one HiveConfig supported per cluster and must be named 'hive'")
		return reconcile.Result{}, nil
	}

	// NOTE: ignoring the Namespace that seems to get set on request when syncing on namespaced objects,
	// when our HiveConfig is ClusterScoped.
	err := r.Get(context.TODO(), types.NamespacedName{Name: request.NamespacedName.Name}, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			hLog.Debug("HiveConfig not found, deleted?")
			r.secretWatchEstablished = false
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		hLog.WithError(err).Error("error reading HiveConfig")
		return reconcile.Result{}, err
	}

	origHiveConfig := instance.DeepCopy()
	hiveNSName := getHiveNamespace(instance)

	if err := r.establishSecretWatch(hLog, hiveNSName); err != nil {
		return reconcile.Result{}, err
	}

	recorder := events.NewRecorder(r.kubeClient.CoreV1().Events(r.hiveOperatorNamespace), "hive-operator", &corev1.ObjectReference{
		Name:      request.Name,
		Namespace: r.hiveOperatorNamespace,
	})

	// Ensure the target namespace for hive components exists and create if not:
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
				err := r.updateHiveConfigStatus(origHiveConfig, instance, cmLog, true)
				if err != nil {
					cmLog.WithError(err).Error("cannot update hash in config status")
				}
				return reconcile.Result{}, err
			}
			cmLog.Debug("configmap unchanged, nothing to do")
		}
	}

	h, err := resource.NewHelperFromRESTConfig(r.restConfig, hLog)
	if err != nil {
		hLog.WithError(err).Error("error creating resource helper")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	managedDomainsConfigMap, err := r.configureManagedDomains(hLog, instance)
	if err != nil {
		hLog.WithError(err).Error("error setting up managed domains")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	plConfigHash, err := r.deployAWSPrivateLinkConfigMap(hLog, h, instance)
	if err != nil {
		hLog.WithError(err).Error("error deploying aws privatelink configmap")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	scConfigHash, err := r.deploySupportedContractsConfigMap(hLog, h, instance)
	if err != nil {
		hLog.WithError(err).Error("error deploying supported contracts configmap")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	confighash, err := r.deployHiveControllersConfigMap(hLog, h, instance, plConfigHash)
	if err != nil {
		hLog.WithError(err).Error("error deploying controllers configmap")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	fgConfigHash, err := r.deployFeatureGatesConfigMap(hLog, h, instance)
	if err != nil {
		hLog.WithError(err).Error("error deploying feature gates configmap")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	err = r.deployHive(hLog, h, instance, recorder, managedDomainsConfigMap, confighash)
	if err != nil {
		hLog.WithError(err).Error("error deploying Hive")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	err = r.deployClusterSync(hLog, h, instance, confighash)
	if err != nil {
		hLog.WithError(err).Error("error deploying ClusterSync")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	// Cleanup legacy objects:
	if err := r.cleanupLegacyObjects(hLog); err != nil {
		return reconcile.Result{}, err
	}

	err = r.deployHiveAdmission(hLog, h, instance, recorder, managedDomainsConfigMap, fgConfigHash, plConfigHash, scConfigHash)
	if err != nil {
		hLog.WithError(err).Error("error deploying HiveAdmission")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	if err := r.cleanupLegacySyncSetInstances(hLog); err != nil {
		hLog.WithError(err).Error("error cleaning up legacy SyncSetInstances")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	if err := r.updateHiveConfigStatus(origHiveConfig, instance, hLog, true); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHiveConfig) establishSecretWatch(hLog *log.Entry, hiveNSName string) error {
	// We need to establish a watch on Secret in the Hive namespace, one time only. We do not know this namespace until
	// we have a HiveConfig.
	if !r.secretWatchEstablished {
		hLog.WithField("namespace", hiveNSName).Info("establishing watch on secrets in hive namespace")

		// Create an informer that only listens to events in the OpenShift managed namespace
		kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(r.kubeClient, watchResyncInterval,
			kubeinformers.WithNamespace(hiveNSName))
		secretsInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
		r.hiveSecretLister = kubeInformerFactory.Core().V1().Secrets().Lister()
		if err := r.mgr.Add(&informerRunnable{informer: secretsInformer}); err != nil {
			hLog.WithError(err).Error("error adding secret informer to manager")
			return err
		}

		// Watch Secrets in hive namespace, so we can detect changes to the hiveadmission serving cert secret and
		// force a deployment rollout.
		err := r.ctrlr.Watch(&source.Informer{Informer: secretsInformer}, handler.Funcs{
			CreateFunc: func(e event.CreateEvent, q workqueue.RateLimitingInterface) {
				hLog.Debug("eventHandler CreateFunc")
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: hiveConfigName}})
			},
			UpdateFunc: func(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
				hLog.Debug("eventHandler UpdateFunc")
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: hiveConfigName}})
			},
		}, predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				hLog.WithField("predicateResponse", e.Object.GetName() == hiveAdmissionServingCertSecretName).Debug("secret CreateEvent")
				return e.Object.GetName() == hiveAdmissionServingCertSecretName
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				hLog.WithField("predicateResponse", e.ObjectNew.GetName() == hiveAdmissionServingCertSecretName).Debug("secret UpdateEvent")
				return e.ObjectNew.GetName() == hiveAdmissionServingCertSecretName
			},
		})
		if err != nil {
			hLog.WithError(err).Error("error establishing secret watch")
			return err
		}

		// Watch secrets in HiveConfig that should trigger reconcile on change.
		err = r.ctrlr.Watch(&source.Informer{Informer: secretsInformer}, handler.EnqueueRequestsFromMapFunc(func(a client.Object) []reconcile.Request {
			retval := []reconcile.Request{}
			secret, ok := a.(*corev1.Secret)
			if !ok {
				// Wasn't a Secret, bail out. This should not happen.
				hLog.Errorf("Error converting MapObject.Object to Secret. Value: %+v", a)
				return retval
			}

			configWithSecrets := &hivev1.HiveConfigList{}
			err := r.mgr.GetClient().List(context.Background(), configWithSecrets, client.MatchingFields{"spec.secrets.secretName": secret.Name})
			if err != nil {
				hLog.Errorf("Error listing HiveConfigs for secret %s: %v", secret.Name, err)
				return retval
			}
			for _, config := range configWithSecrets.Items {
				retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
					Name: config.Name,
				}})
			}
			hLog.WithField("secretName", secret.Name).WithField("configs", retval).Debug("secret change trigger reconcile for HiveConfigs")
			return retval
		}))
		if err != nil {
			return err
		}

		r.secretWatchEstablished = true
	} else {
		hLog.Debug("secret watch already established")
	}
	return nil
}

func (r *ReconcileHiveConfig) cleanupLegacyObjects(hLog log.FieldLogger) error {
	gvrNSNames := []gvrNSName{
		{group: "rbac.authorization.k8s.io", version: "v1", resource: "clusterroles", name: "manager-role"},
		{group: "rbac.authorization.k8s.io", version: "v1", resource: "clusterrolebindings", name: "manager-rolebinding"},
	}

	for _, gvrnsn := range gvrNSNames {
		if err := dynamicDelete(r.dynamicClient, gvrnsn, hLog); err != nil {
			return err
		}
	}
	return nil
}

type informerRunnable struct {
	informer cache.SharedIndexInformer
}

func (r *informerRunnable) Start(ctx context.Context) error {
	stopch := ctx.Done()
	r.informer.Run(stopch)
	cache.WaitForCacheSync(stopch, r.informer.HasSynced)
	return nil
}

func aggregatorCAConfigMapHandler(o client.Object) []reconcile.Request {
	if o.GetName() == aggregatorCAConfigMapName {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: hiveConfigName}}}
	}
	return nil
}

func computeHash(data map[string]string) string {
	hasher := md5.New()
	hasher.Write([]byte(fmt.Sprintf("%v", data)))
	return hex.EncodeToString(hasher.Sum(nil))
}

func (r *ReconcileHiveConfig) updateHiveConfigStatus(origHiveConfig, newHiveConfig *hivev1.HiveConfig, logger log.FieldLogger, succeeded bool) error {
	newHiveConfig.Status.ObservedGeneration = newHiveConfig.Generation
	newHiveConfig.Status.ConfigApplied = succeeded

	if reflect.DeepEqual(origHiveConfig, newHiveConfig) {
		logger.Debug("HiveConfig unchanged, no update required")
		return nil
	}

	logger.Info("HiveConfig has changed, updating")

	err := r.Status().Update(context.TODO(), newHiveConfig)
	if err != nil {
		logger.WithError(err).Error("failed to update HiveConfig status")
	}
	return err
}
