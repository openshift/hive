package hive

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/resource"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/operator/metrics"
	"github.com/openshift/hive/pkg/operator/util"
)

const (
	ControllerName = hivev1.HiveControllerName

	hiveOperatorDeploymentName = "hive-operator"

	managedConfigNamespace    = "openshift-config-managed"
	aggregatorCAConfigMapName = "kube-apiserver-aggregator-client-ca"

	// HiveOperatorNamespaceEnvVar is the environment variable we expect to be given with the namespace the hive-operator is running in.
	HiveOperatorNamespaceEnvVar = "HIVE_OPERATOR_NS"

	// watchResyncInterval is used for a couple handcrafted watches we do with our own informers.
	watchResyncInterval = 30 * time.Minute

	// targetNamespaceLabel is the key (the value will always be "true") for the label we add to
	// the namespace into which we deploy hive -- i.e. HiveConfig.Spec.TargetNamespace. We use this
	// to identify namespaces that were *previously* the configured TargetNamespace so we can clean
	// them up.
	targetNamespaceLabel = "hive.openshift.io/target-namespace"
)

var (
	// HiveConfigConditions are the HiveConfig conditions controlled by
	// hive controller
	HiveConfigConditions = []hivev1.HiveConfigConditionType{
		hivev1.HiveReadyCondition,
	}
)

// Add creates a new Hive Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileHiveConfig{
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

	r.(*ReconcileHiveConfig).isOpenShift, err = r.(*ReconcileHiveConfig).runningOnOpenShift()
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

	// Determine if the openshift-config-managed namespace exists (OpenShift only). If so, set up a
	// watch for configmaps in that namespace.
	ns := &corev1.Namespace{}
	log.Debugf("checking for existence of the %s namespace", managedConfigNamespace)
	err = tempClient.Get(context.TODO(), types.NamespacedName{Name: managedConfigNamespace}, ns)
	if err != nil && !apierrors.IsNotFound(err) {
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
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.HiveConfig{}), &handler.EnqueueRequestForObject{})
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
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1.DaemonSet{}), handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.HiveConfig{}))
	if err != nil {
		return err
	}

	// Monitor changes to Deployments:
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1.Deployment{}), handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.HiveConfig{}))
	if err != nil {
		return err
	}

	// Monitor changes to Services:
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Service{}), handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.HiveConfig{}))
	if err != nil {
		return err
	}

	// Monitor changes to StatefulSets:
	err = c.Watch(source.Kind(mgr.GetCache(), &appsv1.StatefulSet{}), handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &hivev1.HiveConfig{}))
	if err != nil {
		return err
	}

	mapToHiveConfig := func(src string) func(context.Context, client.Object) []reconcile.Request {
		return func(ctx context.Context, _ client.Object) []reconcile.Request {
			retval := []reconcile.Request{}

			configList := &hivev1.HiveConfigList{}
			err := r.(*ReconcileHiveConfig).List(ctx, configList, "", metav1.ListOptions{}) // reconcile all HiveConfigs
			if err != nil {
				log.WithError(err).Errorf("error listing hive configs for %s reconcile", src)
				return retval
			}

			for _, config := range configList.Items {
				retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
					Name: config.Name,
				}})
			}
			log.WithField("configs", retval).Debugf("reconciled for change in %s", src)
			return retval
		}
	}

	// Monitor CRDs so that we can keep latest list of supported contracts
	err = c.Watch(source.Kind(mgr.GetCache(), &apiextv1.CustomResourceDefinition{}),
		handler.EnqueueRequestsFromMapFunc(mapToHiveConfig("CRD")))
	if err != nil {
		return err
	}

	// If the cluster proxy changes, we'll redeploy with the new values in the controllers' envs.
	// There's just one Proxy object; and there's just one HiveConfig -- map any activity on the
	// former to the latter. Note that Proxy is Openshift-specific.
	if r.(*ReconcileHiveConfig).isOpenShift {
		err = c.Watch(source.Kind(mgr.GetCache(), &configv1.Proxy{}),
			handler.EnqueueRequestsFromMapFunc(mapToHiveConfig("Proxy")))
		if err != nil {
			return err
		}
	}

	// Monitor the hive namespace so we can reconcile labels for monitoring. We do this with a map
	// func instead of an owner reference because we can't be sure we'll be the one to create it.
	err = c.Watch(source.Kind(mgr.GetCache(), &corev1.Namespace{}),
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, o client.Object) []reconcile.Request {
			retval := []reconcile.Request{}
			nsName := o.GetName()

			configList := &hivev1.HiveConfigList{}
			err := r.(*ReconcileHiveConfig).List(context.TODO(), configList, "", metav1.ListOptions{})
			if err != nil {
				log.WithError(err).Errorf("error listing hive configs for namespace %s reconcile", nsName)
				return retval
			}

			shouldEnqueue := func(targetNS string) bool {
				// Always enqueue all HiveConfigs if the operator ns was updated
				if nsName == hiveOperatorNS {
					return true
				}
				// Enqueue all HiveConfigs that point to the namespace that triggered us
				// TODO: Is this default const'ed somewhere?
				if targetNS == "" && nsName == "hive" {
					return true
				}
				return targetNS == nsName
			}

			for _, config := range configList.Items {
				if shouldEnqueue(config.Spec.TargetNamespace) {
					retval = append(retval, reconcile.Request{NamespacedName: types.NamespacedName{
						Name: config.Name,
					}})
				}
			}
			log.WithField("configs", retval).Debugf("reconciled for change in namespace %s", nsName)
			return retval

		}))
	if err != nil {
		return err
	}

	// Lookup the hive-operator Deployment. We will assume hive components should all be
	// using the same image, pull policy, node selector, and tolerations as the operator.
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
		nodeSelector := operatorDeployment.Spec.Template.Spec.NodeSelector
		log.Debugf("loaded nodeSelector from hive-operator deployment: %v", nodeSelector)
		r.(*ReconcileHiveConfig).nodeSelector = nodeSelector
		tolerations := operatorDeployment.Spec.Template.Spec.Tolerations
		log.Debugf("loaded tolerations from hive-operator deployment: %v", tolerations)
		r.(*ReconcileHiveConfig).tolerations = tolerations
	} else {
		log.WithError(err).Fatal("unable to look up hive-operator Deployment")
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
	scheme                            *runtime.Scheme
	kubeClient                        kubernetes.Interface
	discoveryClient                   discovery.DiscoveryInterface
	dynamicClient                     dynamic.Interface
	restConfig                        *rest.Config
	hiveImage                         string
	hiveOperatorNamespace             string
	hiveImagePullPolicy               corev1.PullPolicy
	nodeSelector                      map[string]string
	tolerations                       []corev1.Toleration
	syncAggregatorCA                  bool
	managedConfigCMLister             corev1listers.ConfigMapLister
	ctrlr                             controller.Controller
	hiveSecretLister                  corev1listers.SecretLister
	secretWatchEstablishedInNamespace string
	mgr                               manager.Manager
	isOpenShift                       bool
}

// Reconcile reads that state of the cluster for a Hive object and makes changes based on the state read
// and what is in the Hive.Spec
func (r *ReconcileHiveConfig) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	hLog := log.WithField("controller", "hive")
	hLog.Info("Reconciling Hive components")
	recobsrv := metrics.NewReconcileObserver(ControllerName, hLog)
	defer recobsrv.ObserveControllerReconcileTime()

	// Fetch the Hive instance
	instance := &hivev1.HiveConfig{}

	// We only support one HiveConfig per cluster, and it must be called "hive". This prevents installing
	// Hive more than once in the cluster.
	if request.NamespacedName.Name != constants.HiveConfigName {
		hLog.WithField("hiveConfig", request.NamespacedName.Name).Warn(
			"invalid HiveConfig name, only one HiveConfig supported per cluster and must be named 'hive'")
		return reconcile.Result{}, nil
	}

	// NOTE: ignoring the Namespace that seems to get set on request when syncing on namespaced objects,
	// when our HiveConfig is ClusterScoped.
	err := r.Get(context.TODO(), types.NamespacedName{Name: request.NamespacedName.Name}, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			hLog.Debug("HiveConfig not found, deleted?")
			r.secretWatchEstablishedInNamespace = ""
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		hLog.WithError(err).Error("error reading HiveConfig")
		return reconcile.Result{}, err
	}

	origHiveConfig := instance.DeepCopy()
	hiveNSName := GetHiveNamespace(instance)

	// Initialize HiveConfig conditions if not present
	newConditions, changed := util.InitializeHiveConfigConditions(instance.Status.Conditions, HiveConfigConditions)
	if changed {
		instance.Status.Conditions = newConditions
		hLog.Info("initializing hive controller conditions")
		err = r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	if err := r.establishSecretWatch(hLog, hiveNSName); err != nil {
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorEstablishingSecretWatch", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	h, err := resource.NewHelperFromRESTConfig(r.restConfig, hLog)
	if err != nil {
		hLog.WithError(err).Error("error creating resource helper")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorCreatingResourceHelper", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	// Ensure the target namespace for hive components exists and create if not:
	hiveNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   hiveNSName,
			Labels: map[string]string{targetNamespaceLabel: "true"},
		},
	}
	if _, err := h.CreateOrUpdateRuntimeObject(hiveNamespace, r.scheme); err != nil {
		if apierrors.IsAlreadyExists(err) {
			hLog.WithField("hiveNS", hiveNSName).Debug("target namespace already exists")
		} else {
			hLog.WithError(err).Error("error creating hive target namespace")
			instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorCreatingHiveNamespace", err.Error())
			r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
			return reconcile.Result{}, err
		}
	} else {
		hLog.WithField("hiveNS", hiveNSName).Info("target namespace created")
	}

	// Find all namespaces that we've ever installed into. For each resource installed by this
	// controller into the currently-configured target namespace, we must also ensure that resource
	// is *uninstalled* from any previous target namespaces. Under any reasonable circumstances,
	// this list will have one (the current target) or two (current and one previous, if the config
	// was just changed) items.
	nsList := &corev1.NamespaceList{}
	if err := r.List(context.TODO(), nsList, "", metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=true", targetNamespaceLabel)}); err != nil {
		hLog.WithError(err).Error("error retrieving list of target namespaces")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorListingTargetNamespaces", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}
	var namespacesToClean []string
	for _, ns := range nsList.Items {
		if ns.Name != hiveNSName {
			namespacesToClean = append(namespacesToClean, ns.Name)
		}
	}
	if len(namespacesToClean) > 0 {
		hLog.WithField("namespaces", namespacesToClean).Info("will scrub old target namespaces")
	}

	if r.syncAggregatorCA {
		// We use the configmap lister and not the regular client which only watches resources in the hive namespace
		aggregatorCAConfigMap, err := r.managedConfigCMLister.ConfigMaps(managedConfigNamespace).Get(aggregatorCAConfigMapName)
		// If an error other than not found, retry. If not found, it means we don't need to do anything with
		// admission pods yet.
		cmLog := hLog.WithField("configmap", fmt.Sprintf("%s/%s", managedConfigNamespace, aggregatorCAConfigMapName))
		switch {
		case apierrors.IsNotFound(err):
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

	// We used to name the managed domains configmap dynamically and look it up by label. We
	// stopped doing that, but for upgrade purposes we should continue to look for and delete
	// any such old configmaps for "a while".
	r.scrubOldManagedDomainsConfigMaps(h, hLog, append(namespacesToClean, hiveNSName)...)

	managedDomainsConfigHash, err := r.deployConfigMap(hLog, h, instance, managedDomainsConfigMapInfo, namespacesToClean)
	if err != nil {
		hLog.WithError(err).Error("error setting up managed domains")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorSettingUpManagedDomains", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	plConfigHash, err := r.deployConfigMap(hLog, h, instance, awsPrivateLinkConfigMapInfo, namespacesToClean)
	if err != nil {
		hLog.WithError(err).Error("error deploying aws privatelink configmap")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeployingAWSPrivatelinkConfigmap", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	fpConfigHash, err := r.deployConfigMap(hLog, h, instance, failedProvisionConfigMapInfo, namespacesToClean)
	if err != nil {
		hLog.WithError(err).Error("error deploying failed provision configmap")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeployingFailedProvisionConfigmap", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	mcConfigHash, err := r.deployConfigMap(hLog, h, instance, metricsConfigConfigMapInfo, namespacesToClean)
	if err != nil {
		hLog.WithError(err).Error("error deploying metrics config configmap")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeployingMetricsConfigConfigmap", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	scConfigHash, err := r.deployConfigMap(hLog, h, instance, r.supportedContractsConfigMapInfo(), namespacesToClean)
	if err != nil {
		hLog.WithError(err).Error("error deploying supported contracts configmap")
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	confighash, err := r.deployConfigMap(hLog, h, instance, hiveControllersConfigMapInfo, namespacesToClean)
	if err != nil {
		hLog.WithError(err).Error("error deploying controllers configmap")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeployingControllersConfigmap", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}
	// Incorporate the AWSPrivateLink configmap hash
	confighash = computeHash("", confighash, plConfigHash)

	fgConfigHash, err := r.deployConfigMap(hLog, h, instance, featureGatesConfigMapInfo, namespacesToClean)
	if err != nil {
		hLog.WithError(err).Error("error deploying feature gates configmap")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeployingFeatureGatesConfigmap", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	err = r.deployHive(hLog, h, instance, namespacesToClean, confighash, managedDomainsConfigHash, fpConfigHash, mcConfigHash)
	if err != nil {
		hLog.WithError(err).Error("error deploying Hive")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeployingHive", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	err = r.deployClusterSync(hLog, h, instance, confighash, namespacesToClean)
	if err != nil {
		hLog.WithError(err).Error("error deploying ClusterSync")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeployingClusterSync", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	// Cleanup legacy objects:
	if err := r.cleanupLegacyObjects(hLog); err != nil {
		return reconcile.Result{}, err
	}

	err = r.deployHiveAdmission(hLog, h, instance, namespacesToClean, managedDomainsConfigHash, fgConfigHash, plConfigHash, scConfigHash)
	if err != nil {
		hLog.WithError(err).Error("error deploying HiveAdmission")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeployingHiveAdmission", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	if err := r.cleanupLegacySyncSetInstances(hLog); err != nil {
		hLog.WithError(err).Error("error cleaning up legacy SyncSetInstances")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDeletingLegacySyncSetInstances", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	// If we get here, we've successfully scrubbed all *our* resources out of previous target
	// namespaces. Ideally, we would delete the namespace. Unfortunately, there's no good way to
	// tell whether a) we were the one to create it, or b) it's empty. So settle for unlabeling it,
	// accepting the fact that we might be leaking namespaces.
	for _, ns := range namespacesToClean {
		hLog.Infof("Unlabeling former target namespace %s", ns)
		if err := h.Patch(
			types.NamespacedName{Name: ns}, "Namespace", "v1",
			[]byte(fmt.Sprintf(`{"metadata": {"labels": {"%s": null}}}`, targetNamespaceLabel)), "",
		); err != nil {
			hLog.WithError(err).Errorf("error unlabeling former target namespace %s", ns)
			return reconcile.Result{}, err
		}
	}

	instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionTrue, "DeploymentSuccess", "Hive is deployed successfully")
	if err := r.updateHiveConfigStatus(origHiveConfig, instance, hLog, true); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileHiveConfig) establishSecretWatch(hLog *log.Entry, hiveNSName string) error {
	// We need to establish a watch on Secret in the Hive namespace, one time only. We do not know this namespace until
	// we have a HiveConfig.
	if r.secretWatchEstablishedInNamespace != hiveNSName {
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
			CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.RateLimitingInterface) {
				hLog.Debug("eventHandler CreateFunc")
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: constants.HiveConfigName}})
			},
			UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.RateLimitingInterface) {
				hLog.Debug("eventHandler UpdateFunc")
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: constants.HiveConfigName}})
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
		err = r.ctrlr.Watch(&source.Informer{Informer: secretsInformer}, handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, a client.Object) []reconcile.Request {
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

		r.secretWatchEstablishedInNamespace = hiveNSName
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

func aggregatorCAConfigMapHandler(ctx context.Context, o client.Object) []reconcile.Request {
	if o.GetName() == aggregatorCAConfigMapName {
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: constants.HiveConfigName}}}
	}
	return nil
}

func (r *ReconcileHiveConfig) updateHiveConfigStatus(origHiveConfig, newHiveConfig *hivev1.HiveConfig, logger log.FieldLogger, succeeded bool) error {
	newHiveConfig.Status.ObservedGeneration = newHiveConfig.Generation
	newHiveConfig.Status.ConfigApplied = succeeded

	if reflect.DeepEqual(origHiveConfig, newHiveConfig) {
		logger.Debug("HiveConfig unchanged, no update required")
		return nil
	}

	logger.Info("HiveConfig has changed, updating")

	hiveConfigClient := r.dynamicClient.Resource(hivev1.SchemeGroupVersion.WithResource("hiveconfigs"))
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newHiveConfig)
	if err != nil {
		logger.WithError(err).Error("failed to convert new hive config to unstructured")
		return err
	}
	var usNewHiveConfig unstructured.Unstructured
	usNewHiveConfig.SetUnstructuredContent(content)
	usUpdated, err := hiveConfigClient.UpdateStatus(context.TODO(), &usNewHiveConfig, metav1.UpdateOptions{})
	if err != nil {
		logger.WithError(err).Error("failed to update HiveConfig status")
		return err
	}
	// Simulating Status().Update(), put the updated object back into newHiveConfig
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(usUpdated.UnstructuredContent(), newHiveConfig); err != nil {
		logger.WithError(err).Error("failed to convert unstructured HiveConfig")
	}
	return err
}
