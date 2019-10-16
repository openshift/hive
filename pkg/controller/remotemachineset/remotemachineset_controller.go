package remotemachineset

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"time"

	log "github.com/sirupsen/logrus"

	kapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/openshift/library-go/pkg/operator/resource/resourcemerge"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	awsprovider "sigs.k8s.io/cluster-api-provider-aws/pkg/apis/awsproviderconfig/v1beta1"

	installaws "github.com/openshift/installer/pkg/asset/machines/aws"
	installtypes "github.com/openshift/installer/pkg/types"
	installtypesaws "github.com/openshift/installer/pkg/types/aws"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
	"github.com/openshift/hive/pkg/install"
)

const (
	controllerName       = "remotemachineset"
	adminSSHKeySecretKey = "ssh-publickey"
)

// Add creates a new RemoteMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return AddToManager(mgr, NewReconciler(mgr))
}

// NewReconciler returns a new reconcile.Reconciler
func NewReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileRemoteMachineSet{
		Client:                        controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:                        mgr.GetScheme(),
		logger:                        log.WithField("controller", controllerName),
		remoteClusterAPIClientBuilder: controllerutils.BuildClusterAPIClientFromKubeconfig,
		awsClientBuilder:              awsclient.NewClient,
	}
}

// AddToManager adds a new Controller to mgr with r as the reconcile.Reconciler
func AddToManager(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("remotemachineset-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
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
	remoteClusterAPIClientBuilder func(string, string) (client.Client, error)

	// awsClientBuilder is a function pointer to the function that builds the aws client
	awsClientBuilder func(kClient client.Client, secretName, namespace, region string) (awsclient.Client, error)
}

// Reconcile reads that state of the cluster for a ClusterDeployment object and makes changes to the
// remote cluster MachineSets based on the state read and the worker machines defined in
// ClusterDeployment.Spec.Config.Machines
func (r *ReconcileRemoteMachineSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": request.Name,
		"namespace":         request.Namespace,
	})
	cdLog.Info("reconciling cluster deployment")

	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

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
	if cd.Annotations[constants.SyncsetPauseAnnotation] == "true" {
		log.Warn(constants.SyncsetPauseAnnotation, " is present, hence syncing to cluster is disabled")
		return reconcile.Result{}, nil
	}

	// If the cluster is unreachable, do not reconcile.
	if controllerutils.HasUnreachableCondition(cd) {
		cdLog.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}
	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		cdLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	if cd.Spec.Platform.AWS == nil {
		// TODO: add support for GCP and azure
		cdLog.Warn("skipping machine set management for unsupported cloud platform")
		return reconcile.Result{}, nil
	}

	adminKubeconfigSecret := &kapi.Secret{}
	err = r.Get(context.TODO(), types.NamespacedName{Name: cd.Status.AdminKubeconfigSecret.Name, Namespace: cd.Namespace}, adminKubeconfigSecret)
	if err != nil {
		cdLog.WithError(err).Error("unable to fetch admin kubeconfig secret")
		return reconcile.Result{}, err
	}
	kubeConfig, err := controllerutils.FixupKubeconfigSecretData(adminKubeconfigSecret.Data)
	if err != nil {
		cdLog.WithError(err).Error("unable to fixup admin kubeconfig")
		return reconcile.Result{}, err
	}

	remoteClusterAPIClient, err := r.remoteClusterAPIClientBuilder(string(kubeConfig), controllerName)
	if err != nil {
		cdLog.WithError(err).Error("error building remote cluster-api client connection")
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, r.syncMachineSets(cd, remoteClusterAPIClient, cdLog)
}

func (r *ReconcileRemoteMachineSet) syncMachineSets(
	cd *hivev1.ClusterDeployment,
	remoteClusterAPIClient client.Client,
	cdLog log.FieldLogger) error {

	cdLog.Info("reconciling machine sets for cluster deployment")

	// List MachineSets from remote cluster
	remoteMachineSets := &machineapi.MachineSetList{}
	tm := metav1.TypeMeta{}
	tm.SetGroupVersionKind(machineapi.SchemeGroupVersion.WithKind("MachineSet"))
	err := remoteClusterAPIClient.List(context.Background(), remoteMachineSets, client.UseListOptions(&client.ListOptions{
		Raw: &metav1.ListOptions{TypeMeta: tm},
	}))
	if err != nil {
		cdLog.WithError(err).Error("unable to fetch remote machine sets")
		return err
	}
	cdLog.Infof("found %v remote machine sets", len(remoteMachineSets.Items))

	// Scan the pre-existing machinesets to find an AMI ID we can use if we need to create
	// new machinesets.
	// TODO: this will need work at some point in the future, ideally the AMI should come from
	// release image someday, hopefully we can hold off until that is the case, and look it up when
	// we extract installer image refs.
	var amiID string
	for _, ms := range remoteMachineSets.Items {
		awsProviderSpec, err := decodeAWSMachineProviderSpec(ms.Spec.Template.Spec.ProviderSpec.Value, r.scheme)
		if err != nil {
			cdLog.WithError(err).Warn("error decoding AWSMachineProviderConfig, skipping MachineSet for AMI check")
			continue
		}
		if awsProviderSpec.AMI.ID == nil {
			// Really weird, but keep looking...
			continue
		}
		amiID = *awsProviderSpec.AMI.ID
		cdLog.WithFields(log.Fields{
			"fromRemoteMachineSet": ms.Name,
			"ami":                  amiID,
		}).Debug("resolved AMI to use for new machinesets")
		break
	}
	if amiID == "" {
		return fmt.Errorf("unable to locate AMI to use from pre-existing machine set")
	}

	// Generate expected MachineSets from ClusterDeployment
	generatedMachineSets, err := r.generateMachineSetsFromClusterDeployment(cd, amiID)
	if err != nil {
		cdLog.WithError(err).Error("unable to generate machine sets from cluster deployment")
		return err
	}

	cdLog.Infof("generated %v worker machine sets", len(generatedMachineSets))

	machineSetsToDelete := []machineapi.MachineSet{}
	machineSetsToCreate := []*machineapi.MachineSet{}
	machineSetsToUpdate := []*machineapi.MachineSet{}

	// Find MachineSets that need updating/creating
	for _, ms := range generatedMachineSets {
		found := false
		for _, rMS := range remoteMachineSets.Items {
			if ms.Name == rMS.Name {
				found = true
				objectModified := false
				objectMetaModified := resourcemerge.BoolPtr(false)
				resourcemerge.EnsureObjectMeta(objectMetaModified, &rMS.ObjectMeta, ms.ObjectMeta)
				msLog := cdLog.WithField("machineset", rMS.Name)

				if *rMS.Spec.Replicas != *ms.Spec.Replicas {
					msLog.WithFields(log.Fields{
						"desired":  *ms.Spec.Replicas,
						"observed": *rMS.Spec.Replicas,
					}).Info("replicas out of sync")
					rMS.Spec.Replicas = ms.Spec.Replicas
					objectModified = true
				}

				if !reflect.DeepEqual(rMS.Spec.Template.Spec.Labels, ms.Spec.Template.Spec.Labels) {
					msLog.WithFields(log.Fields{
						"desired":  ms.Spec.Template.Spec.Labels,
						"observed": rMS.Spec.Template.Spec.Labels,
					}).Info("labels out of sync")
					rMS.Spec.Template.Spec.Labels = ms.Spec.Template.Spec.Labels
					objectModified = true
				}

				if !reflect.DeepEqual(rMS.Spec.Template.Spec.Taints, ms.Spec.Template.Spec.Taints) {
					msLog.WithFields(log.Fields{
						"desired":  ms.Spec.Template.Spec.Taints,
						"observed": rMS.Spec.Template.Spec.Taints,
					}).Info("taints out of sync")
					rMS.Spec.Template.Spec.Taints = ms.Spec.Template.Spec.Taints
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
			machineSetsToDelete = append(machineSetsToDelete, rMS)
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
		cdLog.WithField("machineset", ms.Name).Info("updating machineset")
		err = remoteClusterAPIClient.Update(context.Background(), ms)
		if err != nil {
			cdLog.WithError(err).Log(controllerutils.LogLevel(err), "unable to update machine set")
			return err
		}
	}

	for _, ms := range machineSetsToDelete {
		cdLog.WithField("machineset", ms.Name).Info("deleting machineset: ", ms)
		err = remoteClusterAPIClient.Delete(context.Background(), &ms)
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
func (r *ReconcileRemoteMachineSet) generateMachineSetsFromClusterDeployment(cd *hivev1.ClusterDeployment, defaultAMI string) ([]*machineapi.MachineSet, error) {
	generatedMachineSets := []*machineapi.MachineSet{}
	installerMachineSets := []machineapi.MachineSet{}

	// Generate InstallConfig from ClusterDeployment
	ic, err := r.generateInstallConfigFromClusterDeployment(cd)
	if err != nil {
		return nil, err
	}

	// Generate expected MachineSets for Platform from InstallConfig
	workerPools := ic.Compute
	switch ic.Platform.Name() {
	case "aws":
		for _, workerPool := range workerPools {
			if workerPool.Platform.AWS == nil {
				workerPool.Platform.AWS = &installtypesaws.MachinePool{}
			}
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

			icMachineSets, err := installaws.MachineSets(cd.Status.InfraID, ic, &workerPool, defaultAMI, workerPool.Name, "worker-user-data")
			if err != nil {
				return nil, err
			}
			hivePool := findHiveMachinePool(cd, workerPool.Name)
			for _, ms := range icMachineSets {
				// Apply hive MachinePool labels to MachineSet MachineSpec.
				if hivePool.Labels != nil {
					ms.Spec.Template.Spec.ObjectMeta.Labels = map[string]string{}
				}
				for key, value := range hivePool.Labels {
					ms.Spec.Template.Spec.ObjectMeta.Labels[key] = value
				}

				// Apply hive MachinePool taints to MachineSet MachineSpec.
				ms.Spec.Template.Spec.Taints = hivePool.Taints

				// Re-use existing AWS resources for generated MachineSets.
				updateMachineSetAWSMachineProviderConfig(ms, cd.Status.InfraID)
				installerMachineSets = append(installerMachineSets, *ms)
			}
		}
	// TODO: Add other platforms. openstack does not currently support openstack.MachineSets()
	default:
		return nil, fmt.Errorf("invalid platform")
	}
	// Convert installer API []machineapi.MachineSets to []*machineapi.MachineSets
	for _, ms := range installerMachineSets {
		nMS := ms
		generatedMachineSets = append(generatedMachineSets, &nMS)
	}
	return generatedMachineSets, nil
}

// updateMachineSetAWSMachineProviderConfig modifies values in a MachineSet's AWSMachineProviderConfig.
// Currently we modify the AWSMachineProviderConfig IAMInstanceProfile, Subnet and SecurityGroups such that
// the values match the worker pool originally created by the installer.
func updateMachineSetAWSMachineProviderConfig(machineSet *machineapi.MachineSet, infraID string) {
	providerConfig := machineSet.Spec.Template.Spec.ProviderSpec.Value.Object.(*awsprovider.AWSMachineProviderConfig)

	// TODO: assumptions about pre-existing objects by name here is quite dangerous, it's already
	// broken on us once via renames in the installer. We need to start querying for what exists
	// here.
	providerConfig.IAMInstanceProfile = &awsprovider.AWSResourceReference{ID: pointer.StringPtr(fmt.Sprintf("%s-worker-profile", infraID))}
	providerConfig.Subnet = awsprovider.AWSResourceReference{
		Filters: []awsprovider.Filter{{
			Name:   "tag:Name",
			Values: []string{fmt.Sprintf("%s-private-%s", infraID, providerConfig.Placement.AvailabilityZone)},
		}},
	}
	providerConfig.SecurityGroups = []awsprovider.AWSResourceReference{{
		Filters: []awsprovider.Filter{{
			Name:   "tag:Name",
			Values: []string{fmt.Sprintf("%s-worker-sg", infraID)},
		}},
	}}
	machineSet.Spec.Template.Spec.ProviderSpec = machineapi.ProviderSpec{
		Value: &runtime.RawExtension{Object: providerConfig},
	}
}

func findHiveMachinePool(cd *hivev1.ClusterDeployment, poolName string) *hivev1.MachinePool {
	for _, mp := range cd.Spec.Compute {
		if mp.Name == poolName {
			return &mp
		}
	}
	return nil
}

func (r *ReconcileRemoteMachineSet) generateInstallConfigFromClusterDeployment(cd *hivev1.ClusterDeployment) (*installtypes.InstallConfig, error) {
	cdLog := r.logger.WithFields(log.Fields{
		"clusterDeployment": cd.Name,
		"namespace":         cd.Namespace,
	})
	cd = cd.DeepCopy()

	cdLog.Debug("loading SSH key secret")
	sshKey, err := controllerutils.LoadSecretData(r, cd.Spec.SSHKey.Name,
		cd.Namespace, adminSSHKeySecretKey)
	if err != nil {
		cdLog.WithError(err).Error("unable to load ssh key from secret")
		return nil, err
	}

	// Using a fake pull secret here, we don't need the pull secret given our use of
	// this install config, all we're after are the machine pools.
	ic, err := install.GenerateInstallConfig(cd, sshKey, "{}", false)
	if err != nil {
		cdLog.WithError(err).Error("unable to generate install config")
		return nil, err
	}

	return ic, nil
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

func decodeAWSMachineProviderSpec(rawExt *runtime.RawExtension, scheme *runtime.Scheme) (*awsprovider.AWSMachineProviderConfig, error) {
	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDecoder(awsprovider.SchemeGroupVersion)
	if rawExt == nil {
		return nil, fmt.Errorf("MachineSet has no ProviderSpec")
	}
	obj, gvk, err := decoder.Decode([]byte(rawExt.Raw), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decode AWS ProviderConfig: %v", err)
	}
	spec, ok := obj.(*awsprovider.AWSMachineProviderConfig)
	if !ok {
		return nil, fmt.Errorf("Unexpected object: %#v", gvk)
	}
	return spec, nil
}

func encodeAWSMachineProviderSpec(awsProviderSpec *awsprovider.AWSMachineProviderConfig, scheme *runtime.Scheme) (*runtime.RawExtension, error) {

	serializer := jsonserializer.NewSerializer(jsonserializer.DefaultMetaFactory, scheme, scheme, false)
	var buffer bytes.Buffer
	err := serializer.Encode(awsProviderSpec, &buffer)
	if err != nil {
		return nil, err
	}
	return &runtime.RawExtension{
		Raw: buffer.Bytes(),
	}, nil
}
