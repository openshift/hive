/*
Copyright (C) 2019 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package argocdregister provides a controller which ensures provisioned clusters are added
// to the ArgoCD cluster registry, and removed when the cluster is deprovisioned.
package argocdregister

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	ControllerName = "argocdregister"

	argoCDDefaultNamespace   = "argocd"
	argoCDServiceAccountName = "argocd-server"

	adminKubeConfigKey = "kubeconfig"
)

// Add creates a new Argocdregister Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	logger := log.WithField("controller", ControllerName)
	concurrentReconciles, clientRateLimiter, queueRateLimiter, err := controllerutils.GetControllerConfig(mgr.GetClient(), ControllerName)
	if err != nil {
		logger.WithError(err).Error("could not get controller configurations")
		return err
	}
	return AddToManager(mgr, NewReconciler(mgr, logger, clientRateLimiter), concurrentReconciles, queueRateLimiter)
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager, logger log.FieldLogger, rateLimiter flowcontrol.RateLimiter) reconcile.Reconciler {
	r := &ArgoCDRegisterController{
		Client:                 controllerutils.NewClientWithMetricsOrDie(mgr, ControllerName, &rateLimiter),
		scheme:                 mgr.GetScheme(),
		restConfig:             mgr.GetConfig(),
		logger:                 log.WithField("controller", ControllerName),
		tlsClientConfigBuilder: tlsClientConfigBuilderFunc,
	}
	return r
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler, concurrentReconciles int, rateLimiter workqueue.RateLimiter) error {
	// Create a new controller
	c, err := controller.New("argocdregister-controller", mgr, controller.Options{
		Reconciler:              controllerutils.NewDelayingReconciler(r, log.WithField("controller", ControllerName)),
		MaxConcurrentReconciles: concurrentReconciles,
		RateLimiter:             rateLimiter,
	})
	if err != nil {
		log.WithField("controller", ControllerName).WithError(err).Error("could not create controller")
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(source.Kind(mgr.GetCache(), &hivev1.ClusterDeployment{}),
		controllerutils.NewRateLimitedUpdateEventHandler(&handler.EnqueueRequestForObject{}, controllerutils.IsClusterDeploymentErrorUpdateEvent))
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ArgoCDRegisterController{}

// ArgoCDRegisterController reconciles ClusterDeployments and generates ArgoCD cluster secrets for ClusterDeployments
type ArgoCDRegisterController struct {
	client.Client
	scheme                 *runtime.Scheme
	restConfig             *rest.Config
	logger                 log.FieldLogger
	tlsClientConfigBuilder func(clientcmd.ClientConfig, log.FieldLogger) (TLSClientConfig, error)
}

// Reconcile checks if we can establish an API client connection to the remote cluster and maintains the unreachable condition as a result.
func (r *ArgoCDRegisterController) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	cdLog := log.WithFields(log.Fields{
		"clusterDeployment": request.Name,
		"namespace":         request.Namespace,
		"controller":        ControllerName,
	})

	// For logging, we need to see when the reconciliation loop starts and ends.
	cdLog.Info("reconciling cluster deployment")
	recobsrv := hivemetrics.NewReconcileObserver(ControllerName, cdLog)
	defer recobsrv.ObserveControllerReconcileTime()

	argoCDEnabled := os.Getenv(constants.ArgoCDEnvVar)
	if len(argoCDEnabled) == 0 {
		cdLog.Info("ArgoCD integration is not enabled in hive config")
		return reconcile.Result{}, nil
	}

	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}

		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}
	cdLog = controllerutils.AddLogFields(controllerutils.MetaObjectLogTagger{Object: cd}, cdLog)

	if paused, err := strconv.ParseBool(cd.Annotations[constants.ReconcilePauseAnnotation]); err == nil && paused {
		cdLog.Info("skipping reconcile due to ClusterDeployment pause annotation")
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		cdLog.Info("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		cdLog.Error("installed cluster with no cluster metadata")
		return reconcile.Result{}, nil
	}

	return r.reconcileCluster(cd, cdLog)
}

func (r *ArgoCDRegisterController) reconcileCluster(cd *hivev1.ClusterDeployment, cdLog log.FieldLogger) (reconcile.Result, error) {
	// Check for ArgoCDNamespace env as it comes from hive config
	argoCDNamespace := os.Getenv(constants.ArgoCDNamespaceEnvVar)
	if len(argoCDNamespace) == 0 {
		argoCDNamespace = argoCDDefaultNamespace
	}

	if cd.Status.APIURL == "" {
		cdLog.Info("installed cluster does not have Status.APIURL set yet")
		return reconcile.Result{}, fmt.Errorf("installed cluster does not have Status.APIURL set yet")
	}

	// Determine unique and predictable name for the ArgoCD secret.
	clusterSecretName, err := getPredictableSecretName(cd.Status.APIURL)
	if err != nil {
		cdLog.WithError(err).Error("error getting predictable secret name")
		return reconcile.Result{}, err
	}

	// Return early if cluster deployment was deleted
	if !cd.DeletionTimestamp.IsZero() {
		if controllerutil.ContainsFinalizer(cd, hivev1.FinalizerArgoCDCluster) {
			// Clean up secret
			cdLog.Info("deleting ArgoCD cluster secret ", clusterSecretName)
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: clusterSecretName, Namespace: argoCDNamespace},
			}
			if err := r.Delete(context.TODO(), secret); err != nil && !errors.IsNotFound(err) {
				return reconcile.Result{}, fmt.Errorf("failed to delete ArgoCD cluster secret: %w", err)
			}
			// Remove finalizer from cluster deployment
			controllerutil.RemoveFinalizer(cd, hivev1.FinalizerArgoCDCluster)
			if err := r.Update(context.TODO(), cd); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to remove finalizer from cluster deployment: %w", err)
			}
		}
		return reconcile.Result{}, nil
	}

	kubeConfigSecretName := cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name
	argoCDServerConfigBytes, err := r.generateArgoCDServerConfig(kubeConfigSecretName, cd.Namespace, argoCDNamespace, cdLog)
	if err != nil {
		return reconcile.Result{}, err
	}

	data := make(map[string][]byte)
	data["server"] = []byte(cd.Status.APIURL)
	data["name"] = []byte(cd.Name)
	data["config"] = argoCDServerConfigBytes

	argoClusterSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterSecretName,
			Namespace: argoCDNamespace,
			Labels: map[string]string{
				"argocd.argoproj.io/secret-type": "cluster",
				constants.CreatedByHiveLabel:     "true",
			},
		},
		Data: data,
	}

	// Copy all ClusterDeployment labels onto the ArgoCD cluster secret. This will hopefully
	// allow for dynamic generation of ArgoCD Applications (via ArgoCD ApplicationSets).
	for k, v := range cd.Labels {
		argoClusterSecret.Labels[k] = v
	}

	existingArgoCDClusterSecret := &corev1.Secret{}
	err = r.Get(context.Background(), types.NamespacedName{Name: argoClusterSecret.Name, Namespace: argoClusterSecret.Namespace}, existingArgoCDClusterSecret)
	if err != nil && errors.IsNotFound(err) {
		cdLog.Info("creating ArgoCD cluster secret ", argoClusterSecret.Name)
		if err := r.Create(context.TODO(), argoClusterSecret); err != nil && !errors.IsAlreadyExists(err) {
			return reconcile.Result{}, fmt.Errorf("error creating ArgoCD cluster secret %q: %w", argoClusterSecret.Name, err)
		}
	}
	if err == nil {
		changed := false
		if !reflect.DeepEqual(existingArgoCDClusterSecret.Data, argoClusterSecret.Data) {
			existingArgoCDClusterSecret.Data = argoClusterSecret.Data
			changed = true
		}
		if !reflect.DeepEqual(existingArgoCDClusterSecret.Labels, argoClusterSecret.Labels) {
			existingArgoCDClusterSecret.Labels = argoClusterSecret.Labels
			changed = true
		}
		if changed {
			cdLog.Infof("updating ArgoCD cluster secret %s", existingArgoCDClusterSecret.Name)
			err = r.Update(context.Background(), existingArgoCDClusterSecret)
			if err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to update secret %s: %w", existingArgoCDClusterSecret.Name, err)
			}
		}
	}

	// Ensure the cluster deployment has a finalizer for cleanup
	if !controllerutil.ContainsFinalizer(cd, hivev1.FinalizerArgoCDCluster) {
		controllerutil.AddFinalizer(cd, hivev1.FinalizerArgoCDCluster)
	}
	if err := r.Update(context.TODO(), cd); err != nil {
		cdLog.WithError(err).Log(controllerutils.LogLevel(err), "error updating cluster deployment")
		return reconcile.Result{Requeue: true}, nil
	}

	return reconcile.Result{}, nil
}

func (r *ArgoCDRegisterController) loadArgoCDServiceAccountToken(argoCDNamespace string) (string, error) {
	serviceAccount := &corev1.ServiceAccount{}
	err := r.Client.Get(context.Background(),
		types.NamespacedName{
			Name:      argoCDServiceAccountName,
			Namespace: argoCDNamespace,
		}, serviceAccount)
	if err != nil {
		return "", fmt.Errorf("error looking up %s service account: %v", argoCDServiceAccountName, err)
	}
	if len(serviceAccount.Secrets) == 0 {
		return "", fmt.Errorf("%s service account has no secrets", argoCDServiceAccountName)
	}

	secretName := ""
	for _, secret := range serviceAccount.Secrets {
		if strings.Contains(secret.Name, "token") {
			secretName = secret.Name
		}
	}
	if secretName == "" {
		return "", fmt.Errorf("%s service account has no token secret", argoCDServiceAccountName)
	}

	secret := &corev1.Secret{}
	err = r.Client.Get(context.Background(),
		types.NamespacedName{
			Name:      secretName,
			Namespace: argoCDNamespace,
		}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve secret %q: %v", secretName, err)
	}
	token, ok := secret.Data["token"]
	if !ok {
		return "", fmt.Errorf("secret %q for service account %q has no token", secretName, serviceAccount)
	}
	return string(token), nil
}

func (r *ArgoCDRegisterController) generateArgoCDServerConfig(kubeconfigSecretName, kubeConfigSecretNamespace, argoCDNamespace string, cdLog log.FieldLogger) ([]byte, error) {
	kubeconfig, err := r.loadSecretData(kubeconfigSecretName, kubeConfigSecretNamespace, adminKubeConfigKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load cluster admin kubeconfig")
		return nil, err
	}

	managerBearerToken, err := r.loadArgoCDServiceAccountToken(argoCDNamespace)
	if err != nil {
		cdLog.WithError(err).Error("unable to load argocd service account token")
		return nil, err
	}

	// Parse the clusters kubeconfig so we can get the fields we need for argo's config:
	config, err := clientcmd.Load([]byte(kubeconfig))
	if err != nil {
		cdLog.WithError(err).Error("unable to load cluster kubeconfig")
		return nil, err
	}
	kubeConfig := clientcmd.NewDefaultClientConfig(*config, &clientcmd.ConfigOverrides{})
	tlsClientConfig, err := r.tlsClientConfigBuilder(kubeConfig, cdLog)
	if err != nil {
		return nil, err
	}

	// Argo uses a custom format for their server config blob, not a kubeconfig:
	argoCDServerConfig := ClusterConfig{
		BearerToken:     managerBearerToken,
		TLSClientConfig: tlsClientConfig,
	}

	argoCDServerConfigBytes, err := json.Marshal(argoCDServerConfig)
	if err != nil {
		return nil, err
	}
	return argoCDServerConfigBytes, nil
}

func tlsClientConfigBuilderFunc(kubeConfig clientcmd.ClientConfig, cdLog log.FieldLogger) (TLSClientConfig, error) {
	cfg, err := kubeConfig.ClientConfig()
	if err != nil {
		cdLog.WithError(err).Error("unable to load client config")
		return TLSClientConfig{}, err
	}

	tlsClientConfig := TLSClientConfig{
		Insecure:   cfg.TLSClientConfig.Insecure,
		ServerName: cfg.TLSClientConfig.ServerName,
		CAData:     cfg.TLSClientConfig.CAData,
		CertData:   cfg.TLSClientConfig.CertData,
		KeyData:    cfg.TLSClientConfig.KeyData,
	}

	return tlsClientConfig, nil
}

func (r *ArgoCDRegisterController) loadSecretData(secretName, namespace, dataKey string) (string, error) {
	s := &corev1.Secret{}
	err := r.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: namespace}, s)
	if err != nil {
		return "", err
	}
	retStr, ok := s.Data[dataKey]
	if !ok {
		return "", fmt.Errorf("secret %s did not contain key %s", secretName, dataKey)
	}
	return string(retStr), nil
}

// getPredictableSecretName generates a unique secret name by hashing the server API URL,
// which is required as all cluster secrets land in the argocd namespace.
// This code matches what is presently done in ArgoCD (util/db/cluster.go). However the actual
// name of the secret does not matter , so we run limited risk of the implementation changing
// out from underneath us.
func getPredictableSecretName(serverAddr string) (string, error) {
	serverURL, err := url.ParseRequestURI(serverAddr)
	if err != nil {
		return "", err
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(serverAddr))
	host := strings.ToLower(strings.Split(serverURL.Host, ":")[0])
	return fmt.Sprintf("cluster-%s-%v", host, h.Sum32()), nil
}
