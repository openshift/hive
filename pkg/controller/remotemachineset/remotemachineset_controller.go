package remotemachineset

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
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

	awsmachines "github.com/openshift/installer/pkg/asset/machines/aws"
	installertypes "github.com/openshift/installer/pkg/types"
	installeraws "github.com/openshift/installer/pkg/types/aws"
	installerazure "github.com/openshift/installer/pkg/types/azure"
	installergcp "github.com/openshift/installer/pkg/types/gcp"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1/gcp"
	"github.com/openshift/hive/pkg/awsclient"
	"github.com/openshift/hive/pkg/constants"
	hivemetrics "github.com/openshift/hive/pkg/controller/metrics"
	controllerutils "github.com/openshift/hive/pkg/controller/utils"
)

const (
	controllerName = "remotemachineset"

	machinePoolNameLabel = "hive.openshift.io/machine-pool"
	finalizer            = "hive.openshift.io/remotemachineset"
)

// Add creates a new RemoteMachineSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the
// Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	r := &ReconcileRemoteMachineSet{
		Client:                        controllerutils.NewClientWithMetricsOrDie(mgr, controllerName),
		scheme:                        mgr.GetScheme(),
		logger:                        log.WithField("controller", controllerName),
		remoteClusterAPIClientBuilder: controllerutils.BuildClusterAPIClientFromKubeconfig,
		awsClientBuilder:              awsclient.NewClient,
	}

	// Create a new controller
	c, err := controller.New("remotemachineset-controller", mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: controllerutils.GetConcurrentReconciles()})
	if err != nil {
		return err
	}

	// Watch for changes to MachinePools
	err = c.Watch(&source.Kind{Type: &hivev1.MachinePool{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to ClusterDeployment
	err = c.Watch(&source.Kind{Type: &hivev1.ClusterDeployment{}}, &handler.EnqueueRequestsFromMapFunc{
		ToRequests: handler.ToRequestsFunc(r.clusterDeploymentWatchHandler),
	})
	if err != nil {
		return err
	}

	return nil
}

func (r *ReconcileRemoteMachineSet) clusterDeploymentWatchHandler(a handler.MapObject) []reconcile.Request {
	retval := []reconcile.Request{}

	cd := a.Object.(*hivev1.ClusterDeployment)
	if cd == nil {
		// Wasn't a clusterdeployment, bail out. This should not happen.
		log.Errorf("Error converting MapObject.Object to ClusterDeployment. Value: %+v", a.Object)
		return retval
	}

	pools := &hivev1.MachinePoolList{}
	err := r.List(context.TODO(), pools)
	if err != nil {
		// Could not list machine pools
		log.Errorf("Error listing machine pools. Value: %+v", a.Object)
		return retval
	}

	for _, pool := range pools.Items {
		if pool.Spec.ClusterDeploymentRef.Name != cd.Name {
			continue
		}
		key := client.ObjectKey{Namespace: pool.Namespace, Name: pool.Name}
		retval = append(retval, reconcile.Request{NamespacedName: key})
	}

	return retval
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

// Reconcile reads that state of the cluster for a MachinePool object and makes changes to the
// remote cluster MachineSets based on the state read
func (r *ReconcileRemoteMachineSet) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	start := time.Now()
	cdLog := r.logger.WithFields(log.Fields{
		"machinePool": request.Name,
		"namespace":   request.Namespace,
	})
	cdLog.Info("reconciling machine pool")

	defer func() {
		dur := time.Since(start)
		hivemetrics.MetricControllerReconcileTime.WithLabelValues(controllerName).Observe(dur.Seconds())
		cdLog.WithField("elapsed", dur).Info("reconcile complete")
	}()

	// Fetch the MachinePool instance
	pool := &hivev1.MachinePool{}
	if err := r.Get(context.TODO(), request.NamespacedName, pool); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request
		log.WithError(err).Error("error looking up machine pool")
		return reconcile.Result{}, err
	}

	if !controllerutils.HasFinalizer(pool, finalizer) {
		if pool.DeletionTimestamp != nil {
			return reconcile.Result{}, nil
		}
	}

	cd := &hivev1.ClusterDeployment{}
	switch err := r.Get(
		context.TODO(),
		client.ObjectKey{Namespace: pool.Namespace, Name: pool.Spec.ClusterDeploymentRef.Name},
		cd,
	); {
	case errors.IsNotFound(err):
		log.Debug("clusterdeployment does not exist")
		return r.removeFinalizer(pool)
	case err != nil:
		log.WithError(err).Error("error looking up cluster deploymnet")
		return reconcile.Result{}, err
	}

	if cd.Annotations[constants.SyncsetPauseAnnotation] == "true" {
		log.Warn(constants.SyncsetPauseAnnotation, " is present, hence syncing to cluster is disabled")
		return reconcile.Result{}, nil
	}

	// If the clusterdeployment is deleted, do not reconcile.
	if cd.DeletionTimestamp != nil {
		return r.removeFinalizer(pool)
	}

	// If the cluster is unreachable, do not reconcile.
	if controllerutils.HasUnreachableCondition(cd) {
		cdLog.Debug("skipping cluster with unreachable condition")
		return reconcile.Result{}, nil
	}

	if !cd.Spec.Installed {
		// Cluster isn't installed yet, return
		cdLog.Debug("cluster installation is not complete")
		return reconcile.Result{}, nil
	}

	if cd.Spec.ClusterMetadata == nil {
		cdLog.Error("installed cluster with no cluster metadata")
		return reconcile.Result{}, nil
	}

	if cd.Spec.Platform.AWS == nil {
		// TODO: add support for GCP and azure
		cdLog.Warn("skipping machine set management for unsupported cloud platform")
		return reconcile.Result{}, nil
	}

	if !controllerutils.HasFinalizer(pool, finalizer) {
		controllerutils.AddFinalizer(pool, finalizer)
		err := r.Update(context.Background(), pool)
		if err != nil {
			log.WithError(err).Log(controllerutils.LogLevel(err), "could not add finalizer")
		}
		return reconcile.Result{}, err
	}

	adminKubeconfigSecret := &kapi.Secret{}
	if err := r.Get(
		context.TODO(),
		types.NamespacedName{Name: cd.Spec.ClusterMetadata.AdminKubeconfigSecretRef.Name, Namespace: cd.Namespace},
		adminKubeconfigSecret,
	); err != nil {
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

	if err := r.syncMachineSets(pool, cd, remoteClusterAPIClient, cdLog); err != nil {
		return reconcile.Result{}, err
	}

	if pool.DeletionTimestamp != nil {
		return r.removeFinalizer(pool)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileRemoteMachineSet) syncMachineSets(
	pool *hivev1.MachinePool,
	cd *hivev1.ClusterDeployment,
	remoteClusterAPIClient client.Client,
	cdLog log.FieldLogger) error {

	cdLog.Info("reconciling machine pool for cluster deployment")

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

	generatedMachineSets := []*machineapi.MachineSet{}
	machineSetsToDelete := []*machineapi.MachineSet{}
	machineSetsToCreate := []*machineapi.MachineSet{}
	machineSetsToUpdate := []*machineapi.MachineSet{}

	if pool.DeletionTimestamp == nil {
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

		// Generate expected MachineSets for machine pool
		generatedMachineSets, err = r.generateMachineSetsForMachinePool(cd, pool, amiID)
		if err != nil {
			cdLog.WithError(err).Error("unable to generate machine sets for machine pool")
			return err
		}

		cdLog.Infof("generated %v worker machine sets", len(generatedMachineSets))

		// Find MachineSets that need updating/creating
		for _, ms := range generatedMachineSets {
			found := false
			for _, rMS := range remoteMachineSets.Items {
				if ms.Name == rMS.Name {
					found = true
					objectModified := false
					objectMetaModified := false
					resourcemerge.EnsureObjectMeta(&objectMetaModified, &rMS.ObjectMeta, ms.ObjectMeta)
					msLog := cdLog.WithField("machineset", rMS.Name)

					if *rMS.Spec.Replicas != *ms.Spec.Replicas {
						msLog.WithFields(log.Fields{
							"desired":  *ms.Spec.Replicas,
							"observed": *rMS.Spec.Replicas,
						}).Info("replicas out of sync")
						rMS.Spec.Replicas = ms.Spec.Replicas
						objectModified = true
					}

					// Update if the labels on the remote machineset are different than the labels on the generated machineset.
					// If the length of both labels is zero, then they match, even if one is a nil map and the other is an empty map.
					if rl, l := rMS.Spec.Template.Spec.Labels, ms.Spec.Template.Spec.Labels; (len(rl) != 0 || len(l) != 0) && !reflect.DeepEqual(rl, l) {
						msLog.WithField("desired", l).WithField("observed", rl).Info("labels out of sync")
						rMS.Spec.Template.Spec.Labels = l
						objectModified = true
					}

					// Update if the taints on the remote machineset are different than the taints on the generated machineset.
					// If the length of both taints is zero, then they match, even if one is a nil slice and the other is an empty slice.
					if rt, t := rMS.Spec.Template.Spec.Taints, ms.Spec.Template.Spec.Taints; (len(rt) != 0 || len(t) != 0) && !reflect.DeepEqual(rt, t) {
						msLog.WithField("desired", t).WithField("observed", rt).Info("taints out of sync")
						rMS.Spec.Template.Spec.Taints = t
						objectModified = true
					}

					if objectMetaModified || objectModified {
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
	}

	// Find MachineSets that need deleting
	for i, rMS := range remoteMachineSets.Items {
		if !isMachineSetControlledByMachinePool(cd, pool, &rMS) {
			continue
		}
		delete := true
		if pool.DeletionTimestamp == nil {
			for _, ms := range generatedMachineSets {
				if rMS.Name == ms.Name {
					delete = false
					break
				}
			}
		}
		if delete {
			machineSetsToDelete = append(machineSetsToDelete, &remoteMachineSets.Items[i])
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

// generateMachineSetsForMachinePool generates expected MachineSets for a machine pool
// using the installer MachineSets API for the MachinePool Platform.
func (r *ReconcileRemoteMachineSet) generateMachineSetsForMachinePool(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, defaultAMI string) ([]*machineapi.MachineSet, error) {
	generatedMachineSets := []*machineapi.MachineSet{}
	installerMachineSets := []machineapi.MachineSet{}

	// Generate minimal/partial InstallConfig from ClusterDeployment
	ic, err := r.generateInstallConfigForMachinePool(cd, pool)
	if err != nil {
		return nil, err
	}

	// Generate expected MachineSets for Platform from InstallConfig
	workerPool := ic.Compute[0]
	switch ic.Platform.Name() {
	case "aws":
		if workerPool.Platform.AWS == nil {
			workerPool.Platform.AWS = &installeraws.MachinePool{}
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

		icMachineSets, err := awsmachines.MachineSets(cd.Spec.ClusterMetadata.InfraID, ic, &workerPool, defaultAMI, workerPool.Name, "worker-user-data")
		if err != nil {
			return nil, err
		}
		for _, ms := range icMachineSets {
			if ms.Labels == nil {
				ms.Labels = map[string]string{}
			}
			ms.Labels[machinePoolNameLabel] = pool.Spec.Name

			// Apply hive MachinePool labels to MachineSet MachineSpec.
			ms.Spec.Template.Spec.ObjectMeta.Labels = make(map[string]string, len(pool.Spec.Labels))
			for key, value := range pool.Spec.Labels {
				ms.Spec.Template.Spec.ObjectMeta.Labels[key] = value
			}

			// Apply hive MachinePool taints to MachineSet MachineSpec.
			ms.Spec.Template.Spec.Taints = pool.Spec.Taints

			// Re-use existing AWS resources for generated MachineSets.
			updateMachineSetAWSMachineProviderConfig(ms, cd.Spec.ClusterMetadata.InfraID)
			installerMachineSets = append(installerMachineSets, *ms)
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

// generateInstallConfigForMachinePool creates a partial InstallConfig containing just enough of the information we need
// to reconcile machine pools. (most notably the platform information) We cannot load the InstallConfig
// the cluster was created with as this data is meant to be provision time only, all future reconciliation is
// driven by the ClusterDeployment.
func (r *ReconcileRemoteMachineSet) generateInstallConfigForMachinePool(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool) (*installertypes.InstallConfig, error) {
	// TODO: AWS only right now, add GCP and someday Azure
	var ic *installertypes.InstallConfig
	if cd.Spec.Platform.AWS != nil {
		ic = &installertypes.InstallConfig{
			Platform: installertypes.Platform{
				AWS: &installeraws.Platform{
					Region: cd.Spec.Platform.AWS.Region,
				},
			},
			Compute: convertMachinePools(pool),
		}
	} else {
		return nil, fmt.Errorf("unsupported platform for remote machineset management")
	}

	return ic, nil
}

func convertMachinePools(pools ...*hivev1.MachinePool) []installertypes.MachinePool {
	machinePools := make([]installertypes.MachinePool, len(pools))
	for i, mp := range pools {
		machinePools[i] = installertypes.MachinePool{
			Name:     mp.Spec.Name,
			Replicas: mp.Spec.Replicas,
			Platform: *convertMachinePoolPlatform(&mp.Spec.Platform),
		}
	}
	return machinePools
}

func convertMachinePoolPlatform(p *hivev1.MachinePoolPlatform) *installertypes.MachinePoolPlatform {
	return &installertypes.MachinePoolPlatform{
		AWS:   convertAWSMachinePool(p.AWS),
		Azure: convertAzureMachinePool(p.Azure),
		GCP:   convertGCPMachinePool(p.GCP),
	}
}

func convertAWSMachinePool(p *hivev1aws.MachinePoolPlatform) *installeraws.MachinePool {
	if p == nil {
		return nil
	}
	return &installeraws.MachinePool{
		InstanceType: p.InstanceType,
		EC2RootVolume: installeraws.EC2RootVolume{
			IOPS: p.EC2RootVolume.IOPS,
			Size: p.EC2RootVolume.Size,
			Type: p.EC2RootVolume.Type,
		},
		Zones: p.Zones,
	}
}

func convertAzureMachinePool(p *hivev1azure.MachinePool) *installerazure.MachinePool {
	if p == nil {
		return nil
	}
	return &installerazure.MachinePool{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
		OSDisk: installerazure.OSDisk{
			DiskSizeGB: p.OSDisk.DiskSizeGB,
		},
	}
}

func convertGCPMachinePool(p *hivev1gcp.MachinePool) *installergcp.MachinePool {
	if p == nil {
		return nil
	}
	return &installergcp.MachinePool{
		Zones:        p.Zones,
		InstanceType: p.InstanceType,
	}
}

// getAWSClient generates an awsclient
func (r *ReconcileRemoteMachineSet) getAWSClient(cd *hivev1.ClusterDeployment) (awsclient.Client, error) {
	// This allows for using host profiles for AWS auth.
	var secretName, regionName string

	if cd != nil && cd.Spec.Platform.AWS != nil {
		secretName = cd.Spec.Platform.AWS.CredentialsSecretRef.Name
		regionName = cd.Spec.Platform.AWS.Region
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

func isMachineSetControlledByMachinePool(cd *hivev1.ClusterDeployment, pool *hivev1.MachinePool, machineSet *machineapi.MachineSet) bool {
	return strings.HasPrefix(
		machineSet.Name,
		strings.Join([]string{cd.Spec.ClusterName, pool.Spec.Name, ""}, "-"),
	) ||
		machineSet.Labels[machinePoolNameLabel] == pool.Spec.Name
}

func (r *ReconcileRemoteMachineSet) removeFinalizer(pool *hivev1.MachinePool) (reconcile.Result, error) {
	if !controllerutils.HasFinalizer(pool, finalizer) {
		return reconcile.Result{}, nil
	}
	controllerutils.DeleteFinalizer(pool, finalizer)
	err := r.Status().Update(context.Background(), pool)
	if err != nil {
		log.WithError(err).Log(controllerutils.LogLevel(err), "could not remove finalizer")
	}
	return reconcile.Result{}, err
}
