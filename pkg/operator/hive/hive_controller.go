package hive

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
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

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/operator/assets"
	"github.com/openshift/hive/pkg/operator/metrics"
	"github.com/openshift/hive/pkg/operator/util"
	"github.com/openshift/hive/pkg/resource"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
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
	err = c.Watch(&source.Kind{Type: &apiextv1.CustomResourceDefinition{}},
		handler.EnqueueRequestsFromMapFunc(func(_ client.Object) []reconcile.Request {
			retval := []reconcile.Request{}

			configList := &hivev1.HiveConfigList{}
			err := r.(*ReconcileHiveConfig).List(context.TODO(), configList, "", metav1.ListOptions{}) // reconcile all HiveConfigs
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

	// Monitor hive namespaces so we can reconcile labels for monitoring and cleanup. We do this with
	// a map func instead of an owner reference because we can't be sure we'll be the one to create it.
	hcNSName := types.NamespacedName{Name: constants.HiveConfigName}
	hcRequest := reconcile.Request{NamespacedName: hcNSName}
	shouldEnqueue := func(o client.Object, config *hivev1.HiveConfig) bool {
		nsName := o.GetName()
		eLog := log.WithField("namespace", nsName)

		// Always enqueue all HiveConfigs if the operator ns was updated
		if nsName == hiveOperatorNS {
			eLog.Debug("reconciling based on change to operator namespace")
			return true
		}

		// Allow the HiveConfig to be passed in -- this lets us save a REST call for UpdateFunc -- but
		// retrieve it if it wasn't.
		if config == nil {
			config = &hivev1.HiveConfig{}
			err := r.(*ReconcileHiveConfig).Get(context.TODO(), hcNSName, config)
			if err != nil {
				eLog.WithError(err).Error("error getting hive configs for namespace watch")
				return false
			}
		}

		// Use the HiveConfig to see if we're in scale mode; and if not, what the targetNamespace is.
		if config.Spec.ScaleMode {
			if labels := o.GetLabels(); labels != nil {
				if b, _ := strconv.ParseBool(labels[targetNamespaceLabel]); b {
					eLog.Debug("reconciling based on change to target-namespace labeled namespace in scale mode")
					return true
				}
				if b, _ := strconv.ParseBool(labels[constants.DataPlaneNamespaceLabel]); b {
					eLog.Debug("reconciling based on change to data-plane labeled namespace in scale mode")
					return true
				}
			}
			return false
		}
		if config.Spec.TargetNamespace == "" {
			if nsName == constants.DefaultHiveNamespace {
				eLog.Debug("reconciling based on change to default namespace")
				return true
			}
			return false
		}
		if nsName == config.Spec.TargetNamespace {
			eLog.Debug("reconciling based on change to targetNamespace")
			return true
		}
		return false
	}

	err = c.Watch(
		&source.Kind{Type: &corev1.Namespace{}},
		handler.Funcs{
			CreateFunc: func(ce event.CreateEvent, rli workqueue.RateLimitingInterface) {
				log.Debug("namespace watch CreateFunc")
				rli.Add(hcRequest)
			},
			UpdateFunc: func(ue event.UpdateEvent, rli workqueue.RateLimitingInterface) {
				log.Debug("namespace watch UpdateFunc")
				rli.Add(hcRequest)
			},
			DeleteFunc: func(de event.DeleteEvent, rli workqueue.RateLimitingInterface) {
				log.Debug("namespace watch DeleteFunc")
				rli.Add(hcRequest)
			},
			GenericFunc: func(ge event.GenericEvent, rli workqueue.RateLimitingInterface) {
				log.Debug("namespace watch GenericFunc")
				rli.Add(hcRequest)
			},
		},
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return shouldEnqueue(e.Object, nil)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return shouldEnqueue(e.Object, nil)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				config := &hivev1.HiveConfig{}
				err := r.(*ReconcileHiveConfig).Get(context.TODO(), hcNSName, config)
				if err != nil {
					log.WithError(err).WithField("namespace", e.ObjectOld.GetName()).Errorf("error getting hive configs for namespacewatch")
					return false
				}
				return shouldEnqueue(e.ObjectOld, config) || shouldEnqueue(e.ObjectNew, config)
			},
			GenericFunc: func(e event.GenericEvent) bool {
				return shouldEnqueue(e.Object, nil)
			},
		},
	)
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
	scheme                *runtime.Scheme
	kubeClient            kubernetes.Interface
	discoveryClient       discovery.DiscoveryInterface
	dynamicClient         dynamic.Interface
	restConfig            *rest.Config
	hiveImage             string
	hiveOperatorNamespace string
	hiveImagePullPolicy   corev1.PullPolicy
	nodeSelector          map[string]string
	tolerations           []corev1.Toleration
	syncAggregatorCA      bool
	managedConfigCMLister corev1listers.ConfigMapLister
	ctrlr                 controller.Controller
	hiveSecretListers     map[string]corev1listers.SecretNamespaceLister
	mgr                   manager.Manager
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
			r.hiveSecretListers = map[string]corev1listers.SecretNamespaceLister{}
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		hLog.WithError(err).Error("error reading HiveConfig")
		return reconcile.Result{}, err
	}

	origHiveConfig := instance.DeepCopy()

	// Initialize HiveConfig conditions if not present
	newConditions, changed := util.InitializeHiveConfigConditions(instance.Status.Conditions, HiveConfigConditions)
	if changed {
		instance.Status.Conditions = newConditions
		hLog.Info("initializing hive controller conditions")
		err = r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	hiveNSNames, err := r.getHiveNamespaces(instance)
	if err != nil {
		hLog.WithError(err).Error("error discovering target namespaces")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorDiscoveringTargetNamespaces", err.Error())
		if updateErr := r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false); updateErr != nil {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, err
	}

	// Find all namespaces that we've installed into previously. For each resource installed by this
	// controller into the currently-configured target namespace, we must also ensure that resource
	// is *uninstalled* from any previous target namespaces.
	nsList := &corev1.NamespaceList{}
	if err := r.List(context.TODO(), nsList, "", metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=true", targetNamespaceLabel)}); err != nil {
		hLog.WithError(err).Error("error retrieving list of previous target namespaces")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorListingTargetNamespaces", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}
	var namespacesToClean []string
	for _, ns := range nsList.Items {
		if !hiveNSNames.Has(ns.Name) {
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

	h, err := resource.NewHelperFromRESTConfig(r.restConfig, hLog)
	if err != nil {
		hLog.WithError(err).Error("error creating resource helper")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorCreatingResourceHelper", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	if err := r.cleanupLegacyObjects(hLog); err != nil {
		hLog.WithError(err).Error("error cleaning up legacy objects")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorCleaningLegacyObjects", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	deployResults := []hivev1.TargetNamespaceStatus{}
	failCount := 0
	for ns := range hiveNSNames {
		result := r.deployToNamespace(instance, h, ns, namespacesToClean, hLog)
		if result.Result == hivev1.FailureTargetNamespaceDeploymentResult {
			failCount++
		}
		hLog.WithField("targetNamespace", ns).WithField("result", result.Result).Debug("deploy to target namespace")
		deployResults = append(deployResults, result)
		// Clear out namespacesToClean. Each piece of deployToNamespace knows how to scrub its objects out of
		// these namespaces, but we only need to do that once.
		// FIXME: This isn't great, as a failure to scrub namespaces could kill the first namespace deployment.
		namespacesToClean = []string{}
	}

	// Before we record results, deploy the role bindings that need a `subject` per target namespace
	if err := r.deployTargetNSBindings(instance, h, hiveNSNames, hLog); err != nil {
		hLog.WithError(err).Error("error deploying role bindings")
		instance.Status.Conditions = util.SetHiveConfigCondition(instance.Status.Conditions, hivev1.HiveReadyCondition, corev1.ConditionFalse, "ErrorCreatingRoleBindings", err.Error())
		r.updateHiveConfigStatus(origHiveConfig, instance, hLog, false)
		return reconcile.Result{}, err
	}

	// Sort the result list first by status, so the failures are grouped at the top; then by namespace name.
	sort.Slice(deployResults, func(i, j int) bool {
		return deployResults[i].Result < deployResults[j].Result || deployResults[i].Name < deployResults[j].Name
	})
	instance.Status.TargetNamespaces = deployResults

	var msg string
	if failCount > 0 {
		msg = fmt.Sprintf("%d target namespace(s) did not deploy successfully -- see the 'targetNamespaces' status object for details", failCount)
		instance.Status.Conditions = util.SetHiveConfigCondition(
			instance.Status.Conditions,
			hivev1.HiveReadyCondition,
			corev1.ConditionFalse,
			"ErrorDeploying",
			msg)
	} else {
		instance.Status.Conditions = util.SetHiveConfigCondition(
			instance.Status.Conditions,
			hivev1.HiveReadyCondition,
			corev1.ConditionTrue,
			"DeploymentSuccess",
			fmt.Sprintf("%d target namespace(s) deployed successfully", len(deployResults)))
	}
	if err := r.updateHiveConfigStatus(origHiveConfig, instance, hLog, failCount == 0); err != nil {
		return reconcile.Result{}, err
	}

	if failCount > 0 {
		return reconcile.Result{}, errors.New(msg)
	}
	return reconcile.Result{}, nil
}

// getHiveNamespaces returns a set of target namespaces into which hive controllers should be
// deployed.
func (r *ReconcileHiveConfig) getHiveNamespaces(config *hivev1.HiveConfig) (sets.String, error) {
	if config.Spec.TargetNamespace != "" && config.Spec.ScaleMode {
		return nil, fmt.Errorf("TargetNamespace and ScaleMode are mutually exclusive")
	}

	if config.Spec.TargetNamespace == "" && !config.Spec.ScaleMode {
		return sets.NewString(constants.DefaultHiveNamespace), nil
	}

	if config.Spec.TargetNamespace != "" {
		return sets.NewString(config.Spec.TargetNamespace), nil
	}

	nsList := &corev1.NamespaceList{}
	if err := r.List(context.TODO(), nsList, "", metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=true", constants.DataPlaneNamespaceLabel)}); err != nil {
		return nil, err
	}
	ret := sets.NewString()
	for _, ns := range nsList.Items {
		ret.Insert(ns.Name)
	}
	return ret, nil
}

func targetNamespaceStatus(ns string, err error, reason hivev1.TargetNamespaceDeploymentReason, hLog *log.Entry) hivev1.TargetNamespaceStatus {
	ret := hivev1.TargetNamespaceStatus{
		Name:               ns,
		Reason:             reason,
		LastTransitionTime: metav1.Now(),
	}
	if err == nil {
		ret.Result = hivev1.SuccessTargetNamespaceDeploymentResult
		if reason == "" {
			ret.Reason = "DeploymentSuccess"
		}
		ret.Message = "Namespace deployed successfully"
		hLog.Info(ret.Message)
	} else {
		ret.Result = hivev1.FailureTargetNamespaceDeploymentResult
		ret.Message = err.Error()
		hLog.WithError(err).WithField("reason", reason).Error("Namespace deployment failed")
	}
	return ret
}

func (r *ReconcileHiveConfig) deployToNamespace(instance *hivev1.HiveConfig, h resource.Helper, hiveNSName string, namespacesToClean []string, hLog *log.Entry) hivev1.TargetNamespaceStatus {
	// TODO: Plumb in DataPlaneKubeConfigSecret
	// - Look for the secret; fail the deploy if not found
	// - Tell the deployments about it

	hLog = hLog.WithField("targetNamespace", hiveNSName)

	if err := r.establishSecretWatch(hLog, hiveNSName); err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorEstablishingSecretWatch", hLog)
	}

	// Ensure the target namespace for hive components exists and create if not:
	hiveNamespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   hiveNSName,
			Labels: map[string]string{targetNamespaceLabel: "true"},
		},
	}
	if _, err := h.CreateOrUpdateRuntimeObject(hiveNamespace, r.scheme); err != nil {
		// TODO: CreateOrUpdateRuntimeObject doesn't produce an IsAlreadyExists error -- it just "succeeds".
		// So we get the "target namespace created" message every time we hit this chunk.
		if apierrors.IsAlreadyExists(err) {
			hLog.Debug("target namespace already exists")
		} else {
			return targetNamespaceStatus(hiveNSName, err, "ErrorCreatingHiveNamespace", hLog)
		}
	} else {
		hLog.Info("target namespace created")
	}

	// Preflight check: in scale mode, the data plane kubeconfig secret must exist or we will refuse to deploy.
	if instance.Spec.ScaleMode {
		dpkcs := &corev1.Secret{}
		if err := r.Get(context.TODO(), types.NamespacedName{Namespace: hiveNSName, Name: constants.DataPlaneKubeconfigSecretName}, dpkcs); err != nil {
			var reason hivev1.TargetNamespaceDeploymentReason = "ErrorFindingDataPlaneKubeConfigSecret"
			// Give 404 a special reason code
			if apierrors.IsNotFound(err) {
				reason = "MissingDataPlaneKubeConfigSecret"
			}
			return targetNamespaceStatus(hiveNSName, err, reason, hLog)
		}
		// We're counting on the kubeconfig living in an element called `kubeconfig`
		if _, exists := dpkcs.Data["kubeconfig"]; !exists {
			return targetNamespaceStatus(hiveNSName, errors.New("data plane kubeconfig secret does not contain a 'kubeconfig' key"), "InvalidDataPlaneKubeConfigSecret", hLog)
		}
	}

	// We used to name the managed domains configmap dynamically and look it up by label. We
	// stopped doing that, but for upgrade purposes we should continue to look for and delete
	// any such old configmaps for "a while".
	r.scrubOldManagedDomainsConfigMaps(h, hLog, append(namespacesToClean, hiveNSName)...)

	managedDomainsConfigHash, err := r.deployConfigMap(hLog, h, instance, managedDomainsConfigMapInfo, hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorSettingUpManagedDomains", hLog)
	}

	plConfigHash, err := r.deployConfigMap(hLog, h, instance, awsPrivateLinkConfigMapInfo, hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingAWSPrivatelinkConfigmap", hLog)
	}

	fpConfigHash, err := r.deployConfigMap(hLog, h, instance, failedProvisionConfigMapInfo, hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingFailedProvisionConfigmap", hLog)
	}

	mcConfigHash, err := r.deployConfigMap(hLog, h, instance, metricsConfigConfigMapInfo, hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingMetricsConfigConfigmap", hLog)
	}

	scConfigHash, err := r.deployConfigMap(hLog, h, instance, r.supportedContractsConfigMapInfo(), hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingSupportedContractsConfigMap", hLog)
	}

	confighash, err := r.deployConfigMap(hLog, h, instance, hiveControllersConfigMapInfo, hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingControllersConfigmap", hLog)
	}
	// Incorporate the AWSPrivateLink configmap hash
	confighash = computeHash("", confighash, plConfigHash)

	fgConfigHash, err := r.deployConfigMap(hLog, h, instance, featureGatesConfigMapInfo, hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingFeatureGatesConfigmap", hLog)
	}

	err = r.deployHive(hLog, h, instance, hiveNSName, namespacesToClean, confighash, managedDomainsConfigHash, fpConfigHash, mcConfigHash)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingHive", hLog)
	}

	err = r.deployClusterSync(hLog, h, instance, confighash, hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingClusterSync", hLog)
	}

	err = r.reconcileMonitoring(hLog, h, instance, hiveNSName, namespacesToClean)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingMonitoring", hLog)
	}

	err = r.deployHiveAdmission(hLog, h, instance, hiveNSName, namespacesToClean, managedDomainsConfigHash, fgConfigHash, plConfigHash, scConfigHash)
	if err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeployingHiveAdmission", hLog)
	}

	if err := r.cleanupLegacySyncSetInstances(hLog); err != nil {
		return targetNamespaceStatus(hiveNSName, err, "ErrorDeletingLegacySyncSetInstances", hLog)
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
			// TODO: We really have no good way of returning the error. Since this namespace didn't
			// get unlabeled, it will still end up in namespacesToClean the next time we reconcile --
			// but unless something else fails, that may not happen for "a long time". Currently
			// accepting this risk.
		}
	}

	// Success
	return targetNamespaceStatus(hiveNSName, nil, "", hLog)
}

// Apply global ClusterRoleBindings which may need Subject namespace overrides for their ServiceAccounts.
func (r *ReconcileHiveConfig) deployTargetNSBindings(instance *hivev1.HiveConfig, h resource.Helper, hiveNSNames sets.String, hLog log.FieldLogger) error {
	clusterRoleBindingAssets := []string{
		"config/controllers/hive_controllers_role_binding.yaml",
		"config/hiveadmission/hiveadmission_rbac_role_binding.yaml",
	}
	for _, crbAsset := range clusterRoleBindingAssets {

		rb := resourceread.ReadClusterRoleBindingV1OrDie(assets.MustAsset(crbAsset))
		// We're coding this counting on the template having exactly one entry in `subjects`, and it being
		// the shape we're looking for. Safeguard against that changing in the future (this code will need
		// to be reworked).
		if len(rb.Subjects) != 1 || rb.Subjects[0].Kind != "ServiceAccount" {
			return errors.New("RoleBinding template changed in a way I don't know how to handle!")
		}
		// Repeat the (singular) subject in the CRB template for each target namespace
		subjectTemplate := rb.Subjects[0]
		rb.Subjects = make([]rbacv1.Subject, len(hiveNSNames))
		for i, ns := range hiveNSNames.List() {
			rb.Subjects[i] = subjectTemplate
			rb.Subjects[i].Namespace = ns
		}
		if _, err := util.ApplyRuntimeObjectWithGC(h, rb, instance); err != nil {
			return errors.Wrapf(err, "unable to apply asset: %s", crbAsset)
		}

		hLog.WithField("asset", crbAsset).Info("applied ClusterRoleRoleBinding asset with target namespace subjects")
	}

	return nil
}

func (r *ReconcileHiveConfig) establishSecretWatch(hLog *log.Entry, hiveNSName string) error {
	// TODO: How do we *un*Watch? https://github.com/kubernetes-sigs/controller-runtime/issues/1884

	// We need to establish a watch on Secrets in a Hive namespace, one time only.
	if r.hiveSecretListers == nil {
		r.hiveSecretListers = map[string]corev1listers.SecretNamespaceLister{}
	}
	if _, exists := r.hiveSecretListers[hiveNSName]; !exists {
		hLog.Info("establishing watch on secrets in target namespace")

		// Create an informer that only listens to events in the OpenShift managed namespace
		kubeInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(r.kubeClient, watchResyncInterval,
			kubeinformers.WithNamespace(hiveNSName))
		secretsInformer := kubeInformerFactory.Core().V1().Secrets().Informer()
		r.hiveSecretListers[hiveNSName] = kubeInformerFactory.Core().V1().Secrets().Lister().Secrets(hiveNSName)
		if err := r.mgr.Add(&informerRunnable{informer: secretsInformer}); err != nil {
			hLog.WithError(err).Error("error adding secret informer to manager")
			return err
		}

		isSecretWeCareAbout := func(name string) bool {
			ret := false
			switch name {
			case hiveAdmissionServingCertSecretName:
				ret = true
			case constants.DataPlaneKubeconfigSecretName:
				ret = true
			}
			return ret
		}

		// Watch Secrets in hive namespace, so we can detect changes to
		// - the hiveadmission serving cert secret
		// - the data plane kubeconfig secret
		// ...and force a deployment rollout.
		err := r.ctrlr.Watch(&source.Informer{Informer: secretsInformer}, handler.Funcs{
			CreateFunc: func(e event.CreateEvent, q workqueue.RateLimitingInterface) {
				hLog.Debug("secret watch CreateFunc")
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: constants.HiveConfigName}})
			},
			UpdateFunc: func(e event.UpdateEvent, q workqueue.RateLimitingInterface) {
				hLog.Debug("secret watch UpdateFunc")
				q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: constants.HiveConfigName}})
			},
		}, predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				ret := isSecretWeCareAbout(e.Object.GetName())
				hLog.WithField("predicateResponse", ret).Debug("secret CreateEvent")
				return ret
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				ret := isSecretWeCareAbout(e.ObjectNew.GetName())
				hLog.WithField("predicateResponse", ret).Debug("secret UpdateEvent")
				return ret
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
		return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: constants.HiveConfigName}}}
	}
	return nil
}

func (r *ReconcileHiveConfig) updateHiveConfigStatus(origHiveConfig, newHiveConfig *hivev1.HiveConfig, logger log.FieldLogger, succeeded bool) error {
	newHiveConfig.Status.ObservedGeneration = newHiveConfig.Generation
	newHiveConfig.Status.ConfigApplied = succeeded

	// Since we always overwrite timestamps in targetNamespaces, strip them out before deciding whether we've changed anything
	if newHiveConfig.Status.TargetNamespaces == nil {
		newHiveConfig.Status.TargetNamespaces = []hivev1.TargetNamespaceStatus{}
	}
	if origHiveConfig.Status.TargetNamespaces == nil {
		origHiveConfig.Status.TargetNamespaces = []hivev1.TargetNamespaceStatus{}
	}
	tns := make([]hivev1.TargetNamespaceStatus, len(newHiveConfig.Status.TargetNamespaces))
	copy(tns, newHiveConfig.Status.TargetNamespaces)
	for i := range newHiveConfig.Status.TargetNamespaces {
		newHiveConfig.Status.TargetNamespaces[i].LastTransitionTime = metav1.Time{}
	}
	for i := range origHiveConfig.Status.TargetNamespaces {
		origHiveConfig.Status.TargetNamespaces[i].LastTransitionTime = metav1.Time{}
	}

	if reflect.DeepEqual(origHiveConfig, newHiveConfig) {
		logger.Debug("HiveConfig unchanged, no update required")
		return nil
	}

	logger.Info("HiveConfig has changed, updating")
	// Restore original TargetNamespaces
	copy(newHiveConfig.Status.TargetNamespaces, tns)

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
