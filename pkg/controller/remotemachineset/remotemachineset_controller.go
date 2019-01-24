/*
Copyright 2018 The Kubernetes Authors.

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

package remotemachineset

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	kapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	capiv1 "sigs.k8s.io/cluster-api/pkg/apis/cluster/v1alpha1"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	installaws "github.com/openshift/installer/pkg/asset/machines/aws"
	installtypes "github.com/openshift/installer/pkg/types"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/awsclient"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
)

const (
	controllerName = "remotemachineset"

	// remoteClusterAPINamespace is the namespace in which the remote cluster-api stores
	// MachineSets
	remoteClusterAPINamespace = "openshift-cluster-api"

	adminKubeConfigKey          = "kubeconfig"
	adminCredsSecretPasswordKey = "password"
	pullSecretKey               = ".dockercfg"
	adminSSHKeySecretKey        = "ssh-publickey"
)

// Add creates a new RemoteMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRemoteMachineSet{
		Client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		logger: log.WithField("controller", controllerName),
		remoteClusterAPIClientBuilder: controllerutils.BuildClusterAPIClientFromKubeconfig,
		awsClientBuilder:              awsclient.NewClient,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("remotemachineset-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileRemoteMachineSet{}

// ReconcileRemoteMachineSet reconciles the MachineSets generated from a ClusterDeployment object
type ReconcileRemoteMachineSet struct {
	client.Client
	scheme *runtime.Scheme

	logger log.FieldLogger

	// remoteClusterAPIClientBuilder is a function pointer to the function that builds a client for the
	// remote cluster's cluster-api
	remoteClusterAPIClientBuilder func(string) (client.Client, error)

	// awsClientBuilder is a function pointer to the function that builds the aws client
	awsClientBuilder func(kClient client.Client, secretName, namespace, region string) (awsclient.Client, error)
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes to the
// remote cluster MachineSets based on the state read and the worker machines defined in
// ClusterDeployment.Spec.Config.Machines
func (r *ReconcileRemoteMachineSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the ClusterDeployment instance
	cd := &hivev1.ClusterDeployment{}
	err := r.Get(context.TODO(), request.NamespacedName, cd)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up cluster deployment")
		return reconcile.Result{}, err
	}

	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})

	if !cd.Status.Installed {
		// Cluster isn't installed yet, return
		cdLog.Info("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	secretName := cd.Status.AdminKubeconfigSecret.Name
	secretData, err := r.loadSecretData(secretName, cd.Namespace, adminKubeConfigKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load admin kubeconfig")
		return reconcile.Result{}, err
	}

	remoteClusterAPIClient, err := r.remoteClusterAPIClientBuilder(secretData)
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, r.syncMachineSets(cd, remoteClusterAPIClient)
}

func (r *ReconcileRemoteMachineSet) syncMachineSets(cd *hivev1.ClusterDeployment, remoteClusterAPIClient client.Client) error {
	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
	cdLog.Info("reconciling machine sets for cluster deployment")

	// List MachineSets from remote cluster
	remoteMachineSets := &capiv1.MachineSetList{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(capiv1.SchemeGroupVersion.WithKind("MachineSet"))
	err := remoteClusterAPIClient.List(context.Background(), &client.ListOptions{
		Raw: &metav1.ListOptions{TypeMeta: tm},
	}, remoteMachineSets)
	if err != nil {
		cdLog.WithError(err).Error("unable to fetch remote machine sets")
		return err
	}
	cdLog.Infof("found %v remote machine sets", len(remoteMachineSets.Items))

	// Generate expected MachineSets from ClusterDeployment
	generatedMachineSets, err := r.generateMachineSetsFromClusterDeployment(cd)
	if err != nil {
		cdLog.WithError(err).Error("unable to generate machine sets from cluster deployment")
		return err
	}
	cdLog.Infof("generated %v worker machine sets", len(generatedMachineSets))

	machineSetsToDelete := []*capiv1.MachineSet{}
	machineSetsToCreate := []*capiv1.MachineSet{}
	machineSetsToUpdate := []*capiv1.MachineSet{}

	// Find MachineSets that need updating/creating
	for _, ms := range generatedMachineSets {
		found := false
		for _, rMS := range remoteMachineSets.Items {
			if ms.Name == rMS.Name {
				found = true
				objectModified := false
				objectMetaModified := resourcemerge.BoolPtr(false)
				resourcemerge.EnsureObjectMeta(objectMetaModified, &rMS.ObjectMeta, ms.ObjectMeta)

				if *rMS.Spec.Replicas != *ms.Spec.Replicas {
					rMS.Spec.Replicas = ms.Spec.Replicas
					objectModified = true
				}

				if *objectMetaModified || objectModified {
					rMS.Generation++
					machineSetsToUpdate = append(machineSetsToUpdate, &rMS)
				}
				break
			}
		}

		if !found {
			machineSetsToCreate = append(machineSetsToCreate, ms)
		}

	}

	// Find MachineSets that need deleting
	for _, rMS := range remoteMachineSets.Items {
		found := false
		for _, ms := range generatedMachineSets {
			if rMS.Name == ms.Name {
				found = true
				break
			}
		}
		if !found {
			machineSetsToDelete = append(machineSetsToDelete, &rMS)
		}
	}

	for _, ms := range machineSetsToCreate {
		cdLog.WithField("machineset", ms.Name).Info("creating machineset")
		err = remoteClusterAPIClient.Create(context.Background(), ms)
		if err != nil {
			cdLog.WithError(err).Error("unable to create machine set")
			return err
		}
	}

	for _, ms := range machineSetsToUpdate {
		cdLog.WithField("machineset", ms.Name).Info("updating machineset: ", ms)
		err = remoteClusterAPIClient.Update(context.Background(), ms)
		if err != nil {
			cdLog.WithError(err).Error("unable to update machine set")
			return err
		}
	}

	for _, ms := range machineSetsToDelete {
		cdLog.WithField("machineset", ms.Name).Info("deleting machineset: ", ms)
		err = remoteClusterAPIClient.Delete(context.Background(), ms)
		if err != nil {
			cdLog.WithError(err).Error("unable to delete machine set")
			return err
		}
	}

	cdLog.Info("done reconciling machine sets for cluster deployment")
	return nil
}

// generateMachineSetsFromClusterDeployment generates expected MachineSets for a ClusterDeployment
// using the installer MachineSets API for the MachinePool Platform.
func (r *ReconcileRemoteMachineSet) generateMachineSetsFromClusterDeployment(cd *hivev1.ClusterDeployment) ([]*capiv1.MachineSet, error) {
	generatedMachineSets := []*capiv1.MachineSet{}
	installerMachineSets := []capiv1.MachineSet{}

	// Generate InstallConfig from ClusterDeployment
	ic, err := r.generateInstallConfigFromClusterDeployment(cd)
	if err != nil {
		return nil, err
	}

	// TODO: once AMIs are referenced in the release image, this field should be
	// removed from our API, both a default, and per machine pool.
	defaultAMI := cd.Spec.AWS.DefaultMachinePlatform.AMIID

	// Generate expected MachineSets for Platform from InstallConfig
	workerPool := workerPool(ic.Machines)
	switch ic.Platform.Name() {
	case "aws":

		if len(workerPool.Platform.AWS.Zones) == 0 {
			awsClient, err := r.getAWSClient(cd)
			if err != nil {
				return nil, err
			}
			azs, err := fetchAvailabilityZones(awsClient, ic.Platform.AWS.Region)
			if err != nil {
				return nil, err
			}
			// Safety net from deleting machine sets. Do we expect to return 0 availability zones successfully?
			if len(azs) == 0 {
				return nil, fmt.Errorf("fetched 0 availability zones")
			}
			workerPool.Platform.AWS.Zones = azs
		}

		hivePool := findHiveMachinePool(cd, workerPool.Name)
		osImage := hivePool.Platform.AWS.AMIID
		if osImage == "" {
			// A default AMI should be set by the clusterdeployment controller,
			// return error if no AMI has been set
			if defaultAMI == "" {
				return nil, fmt.Errorf("cluster deployment has no default AMI")
			}
			osImage = defaultAMI
		}
		installerMachineSets, err = installaws.MachineSets(cd.Status.ClusterID, ic, &workerPool, osImage, "worker", "worker-user-data")
		if err != nil {
			return nil, err
		}
	// TODO: Add other platforms. openstack does not currently support openstack.MachineSets()
	default:
		return nil, fmt.Errorf("invalid platform")
	}
	// Convert installer API []capiv1.MachineSets to []*capiv1.MachineSets
	for _, ms := range installerMachineSets {
		nMS := ms
		generatedMachineSets = append(generatedMachineSets, &nMS)
	}
	return generatedMachineSets, nil
}

func findHiveMachinePool(cd *hivev1.ClusterDeployment, poolName string) *hivev1.MachinePool {
	for _, mp := range cd.Spec.Compute {
		if mp.Name == poolName {
			return &mp
		}
	}
	return nil
}

func workerPool(pools []installtypes.MachinePool) installtypes.MachinePool {
	for idx, pool := range pools {
		if pool.Name == "worker" {
			return pools[idx]
		}
	}
	return installtypes.MachinePool{}
}

func (r *ReconcileRemoteMachineSet) generateInstallConfigFromClusterDeployment(cd *hivev1.ClusterDeployment) (*installtypes.InstallConfig, error) {
	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
	cd = cd.DeepCopy()

	cdLog.Debug("loading SSH key secret")
	sshKey, err := r.loadSecretData(cd.Spec.SSHKey.Name,
		cd.Namespace, adminSSHKeySecretKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load ssh key from secret")
		return nil, err
	}

	cdLog.Debug("loading pull secret secret")
	pullSecret, err := r.loadSecretData(cd.Spec.PullSecret.Name, cd.Namespace, pullSecretKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load pull secret from secret")
		return nil, err
	}

	ic, err := install.GenerateInstallConfig(cd, sshKey, pullSecret)
	if err != nil {
		cdLog.WithError(err).Error("unable to generate install config")
		return nil, err
	}

	return ic, nil
}

func (r *ReconcileRemoteMachineSet) loadSecretData(secretName, namespace, dataKey string) (string, error) {
	s := &kapi.Secret{}
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

// getAWSClient generates an awsclient
func (r *ReconcileRemoteMachineSet) getAWSClient(cd *hivev1.ClusterDeployment) (awsclient.Client, error) {
	// This allows for using host profiles for AWS auth.
	var secretName, regionName string

	if cd != nil && cd.Spec.AWS != nil && cd.Spec.PlatformSecrets.AWS != nil {
		secretName = cd.Spec.PlatformSecrets.AWS.Credentials.Name
		regionName = cd.Spec.AWS.Region
	}

	awsClient, err := r.awsClientBuilder(r.Client, secretName, cd.Namespace, regionName)
	if err != nil {
		return nil, err
	}

	return awsClient, nil
}

// fetchAvailabilityZones fetches availability zones for the specified region
func fetchAvailabilityZones(client awsclient.Client, region string) ([]string, error) {
	zoneFilter := &ec2.Filter{
		Name:   aws.String("region-name"),
		Values: []*string{aws.String(region)},
	}
	req := &ec2.DescribeAvailabilityZonesInput{
		Filters: []*ec2.Filter{zoneFilter},
	}
	resp, err := client.DescribeAvailabilityZones(req)
	if err != nil {
		return nil, err
	}
	zones := []string{}
	for _, zone := range resp.AvailabilityZones {
		zones = append(zones, *zone.ZoneName)
	}
	return zones, nil
}
