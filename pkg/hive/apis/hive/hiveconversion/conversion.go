package hiveconversion

import (
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1/gcp"

	hiveapi "github.com/openshift/hive/pkg/hive/apis/hive"
	hiveapiaws "github.com/openshift/hive/pkg/hive/apis/hive/aws"
	hiveapiazure "github.com/openshift/hive/pkg/hive/apis/hive/azure"
	hiveapigcp "github.com/openshift/hive/pkg/hive/apis/hive/gcp"
)

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addConversionFuncs)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addConversionFuncs(scheme *runtime.Scheme) error {
	if err := scheme.AddConversionFuncs(
		Convert_v1alpha1_Checkpoint_To_v1_Checkpoint,
		Convert_v1_Checkpoint_To_v1alpha1_Checkpoint,
		Convert_v1alpha1_ClusterDeployment_To_v1_ClusterDeployment,
		Convert_v1_ClusterDeployment_To_v1alpha1_ClusterDeployment,
		Convert_v1alpha1_ClusterDeprovisionRequest_To_v1_ClusterDeprovision,
		Convert_v1_ClusterDeprovision_To_v1alpha1_ClusterDeprovisionRequest,
		Convert_v1alpha1_ClusterImageSet_To_v1_ClusterImageSet,
		Convert_v1_ClusterImageSet_To_v1alpha1_ClusterImageSet,
		Convert_v1alpha1_ClusterProvision_To_v1_ClusterProvision,
		Convert_v1_ClusterProvision_To_v1alpha1_ClusterProvision,
		Convert_v1alpha1_ClusterState_To_v1_ClusterState,
		Convert_v1_ClusterState_To_v1alpha1_ClusterState,
		Convert_v1alpha1_DNSZone_To_v1_DNSZone,
		Convert_v1_DNSZone_To_v1alpha1_DNSZone,
		Convert_v1alpha1_HiveConfig_To_v1_HiveConfig,
		Convert_v1_HiveConfig_To_v1alpha1_HiveConfig,
		Convert_v1alpha1_SyncIdentityProvider_To_v1_SyncIdentityProvider,
		Convert_v1_SyncIdentityProvider_To_v1alpha1_SyncIdentityProvider,
		Convert_v1alpha1_SelectorSyncIdentityProvider_To_v1_SelectorSyncIdentityProvider,
		Convert_v1_SelectorSyncIdentityProvider_To_v1alpha1_SelectorSyncIdentityProvider,
		Convert_v1alpha1_SyncSet_To_v1_SyncSet,
		Convert_v1_SyncSet_To_v1alpha1_SyncSet,
		Convert_v1alpha1_SelectorSyncSet_To_v1_SelectorSyncSet,
		Convert_v1_SelectorSyncSet_To_v1alpha1_SelectorSyncSet,
		Convert_v1alpha1_SyncSetInstance_To_v1_SyncSetInstance,
		Convert_v1_SyncSetInstance_To_v1alpha1_SyncSetInstance,
	); err != nil { // If one of the conversion functions is malformed, detect it immediately.
		return err
	}
	return nil
}

func Convert_v1alpha1_Checkpoint_To_v1_Checkpoint(in *hiveapi.Checkpoint, out *hivev1.Checkpoint, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.LastBackupChecksum = in.Spec.LastBackupChecksum
	out.Spec.LastBackupTime = in.Spec.LastBackupTime
	out.Spec.LastBackupRef.Name = in.Spec.LastBackupRef.Name
	out.Spec.LastBackupRef.Namespace = in.Spec.LastBackupRef.Namespace
	return nil
}

func Convert_v1_Checkpoint_To_v1alpha1_Checkpoint(in *hivev1.Checkpoint, out *hiveapi.Checkpoint, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.LastBackupChecksum = in.Spec.LastBackupChecksum
	out.Spec.LastBackupTime = in.Spec.LastBackupTime
	out.Spec.LastBackupRef.Name = in.Spec.LastBackupRef.Name
	out.Spec.LastBackupRef.Namespace = in.Spec.LastBackupRef.Namespace
	return nil
}

func Convert_v1alpha1_ClusterDeployment_To_v1_ClusterDeployment(in *hiveapi.ClusterDeployment, out *hivev1.ClusterDeployment, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ClusterName = in.Spec.ClusterName
	out.Spec.BaseDomain = in.Spec.BaseDomain
	if inAWS := in.Spec.Platform.AWS; inAWS != nil {
		if out.Spec.Platform.AWS == nil {
			out.Spec.Platform.AWS = &hivev1aws.Platform{}
		}
		outAWS := out.Spec.Platform.AWS
		outAWS.Region = inAWS.Region
		outAWS.UserTags = inAWS.UserTags
		if creds := in.Spec.PlatformSecrets.AWS; creds != nil {
			outAWS.CredentialsSecretRef = creds.Credentials
		} else {
			outAWS.CredentialsSecretRef = corev1.LocalObjectReference{}
		}
	} else {
		out.Spec.Platform.AWS = nil
	}
	if inAzure := in.Spec.Platform.Azure; inAzure != nil {
		if out.Spec.Platform.Azure == nil {
			out.Spec.Platform.Azure = &hivev1azure.Platform{}
		}
		outAzure := out.Spec.Platform.Azure
		outAzure.Region = inAzure.Region
		outAzure.BaseDomainResourceGroupName = inAzure.BaseDomainResourceGroupName
		if creds := in.Spec.PlatformSecrets.Azure; creds != nil {
			outAzure.CredentialsSecretRef = creds.Credentials
		} else {
			outAzure.CredentialsSecretRef = corev1.LocalObjectReference{}
		}
	} else {
		out.Spec.Platform.Azure = nil
	}
	if inGCP := in.Spec.Platform.GCP; inGCP != nil {
		if out.Spec.Platform.GCP == nil {
			out.Spec.Platform.GCP = &hivev1gcp.Platform{}
		}
		outGCP := out.Spec.Platform.GCP
		outGCP.Region = inGCP.Region
		if creds := in.Spec.PlatformSecrets.GCP; creds != nil {
			outGCP.CredentialsSecretRef = creds.Credentials
		} else {
			outGCP.CredentialsSecretRef = corev1.LocalObjectReference{}
		}
	} else {
		out.Spec.Platform.GCP = nil
	}
	out.Spec.PullSecretRef = in.Spec.PullSecret
	out.Spec.PreserveOnDelete = in.Spec.PreserveOnDelete
	out.Spec.ControlPlaneConfig.ServingCertificates.Default = in.Spec.ControlPlaneConfig.ServingCertificates.Default
	if additional := in.Spec.ControlPlaneConfig.ServingCertificates.Additional; additional != nil {
		out.Spec.ControlPlaneConfig.ServingCertificates.Additional = make([]hivev1.ControlPlaneAdditionalCertificate, len(additional))
		for i, inCert := range additional {
			outCert := &out.Spec.ControlPlaneConfig.ServingCertificates.Additional[i]
			outCert.Name = inCert.Name
			outCert.Domain = inCert.Domain
		}
	} else {
		out.Spec.ControlPlaneConfig.ServingCertificates.Additional = nil
	}
	if ingress := in.Spec.Ingress; ingress != nil {
		out.Spec.Ingress = make([]hivev1.ClusterIngress, len(ingress))
		for i, inIngress := range ingress {
			outIngress := &out.Spec.Ingress[i]
			outIngress.Name = inIngress.Name
			outIngress.Domain = inIngress.Domain
			outIngress.NamespaceSelector = inIngress.NamespaceSelector
			outIngress.RouteSelector = inIngress.RouteSelector
			outIngress.ServingCertificate = inIngress.ServingCertificate
		}
	} else {
		out.Spec.Ingress = nil
	}
	if certs := in.Spec.CertificateBundles; certs != nil {
		out.Spec.CertificateBundles = make([]hivev1.CertificateBundleSpec, len(certs))
		for i, inCert := range certs {
			outCert := &out.Spec.CertificateBundles[i]
			outCert.Name = inCert.Name
			outCert.Generate = inCert.Generate
			outCert.CertificateSecretRef = inCert.SecretRef
		}
	} else {
		out.Spec.CertificateBundles = nil
	}
	out.Spec.ManageDNS = in.Spec.ManageDNS
	if in.Status.InfraID != "" {
		out.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
			ClusterID:                in.Status.ClusterID,
			InfraID:                  in.Status.InfraID,
			AdminKubeconfigSecretRef: in.Status.AdminKubeconfigSecret,
			AdminPasswordSecretRef:   in.Status.AdminPasswordSecret,
		}
	} else {
		out.Spec.ClusterMetadata = nil
	}
	out.Spec.Installed = in.Spec.Installed
	if out.Spec.Provisioning == nil {
		out.Spec.Provisioning = &hivev1.Provisioning{}
	}
	out.Spec.Provisioning.ReleaseImage = in.Spec.Images.ReleaseImage
	if is := in.Spec.ImageSet; is != nil {
		out.Spec.Provisioning.ImageSetRef = &hivev1.ClusterImageSetReference{
			Name: is.Name,
		}
	} else {
		out.Spec.Provisioning.ImageSetRef = nil
	}
	if ssh := in.Spec.SSHKey.Name; ssh != "" {
		out.Spec.Provisioning.SSHPrivateKeySecretRef = &corev1.LocalObjectReference{
			Name: ssh,
		}
	} else {
		out.Spec.Provisioning.SSHPrivateKeySecretRef = nil
	}
	out.Status.InstallRestarts = in.Status.InstallRestarts
	out.Status.ClusterVersionStatus = in.Status.ClusterVersionStatus
	out.Status.APIURL = in.Status.APIURL
	out.Status.WebConsoleURL = in.Status.WebConsoleURL
	out.Status.InstallerImage = in.Status.InstallerImage
	out.Status.CLIImage = in.Status.CLIImage
	if conds := in.Status.Conditions; conds != nil {
		out.Status.Conditions = make([]hivev1.ClusterDeploymentCondition, len(conds))
		for i, inCond := range conds {
			outCond := &out.Status.Conditions[i]
			outCond.Type = hivev1.ClusterDeploymentConditionType(inCond.Type)
			outCond.Status = inCond.Status
			outCond.LastProbeTime = inCond.LastProbeTime
			outCond.LastTransitionTime = inCond.LastTransitionTime
			outCond.Reason = inCond.Reason
			outCond.Message = inCond.Message
		}
	} else {
		out.Status.Conditions = nil
	}
	if certs := in.Status.CertificateBundles; certs != nil {
		out.Status.CertificateBundles = make([]hivev1.CertificateBundleStatus, len(certs))
		for i, inCert := range certs {
			outCert := &out.Status.CertificateBundles[i]
			outCert.Name = inCert.Name
			outCert.Generated = inCert.Generated
		}
	} else {
		out.Status.CertificateBundles = nil
	}
	out.Status.InstalledTimestamp = in.Status.InstalledTimestamp
	out.Status.ProvisionRef = in.Status.Provision
	return nil
}

func Convert_v1_ClusterDeployment_To_v1alpha1_ClusterDeployment(in *hivev1.ClusterDeployment, out *hiveapi.ClusterDeployment, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ClusterName = in.Spec.ClusterName
	if p := in.Spec.Provisioning; p != nil && p.SSHPrivateKeySecretRef != nil {
		out.Spec.SSHKey.Name = p.SSHPrivateKeySecretRef.Name
	} else {
		out.Spec.SSHKey.Name = ""
	}
	out.Spec.BaseDomain = in.Spec.BaseDomain
	if inAWS := in.Spec.Platform.AWS; inAWS != nil {
		if out.Spec.Platform.AWS == nil {
			out.Spec.Platform.AWS = &hiveapiaws.Platform{}
		}
		outAWS := out.Spec.Platform.AWS
		outAWS.Region = inAWS.Region
		outAWS.UserTags = inAWS.UserTags
		if out.Spec.PlatformSecrets.AWS == nil {
			out.Spec.PlatformSecrets.AWS = &hiveapiaws.PlatformSecrets{}
		}
		out.Spec.PlatformSecrets.AWS.Credentials = inAWS.CredentialsSecretRef
	} else {
		out.Spec.Platform.AWS = nil
		out.Spec.PlatformSecrets.AWS = nil
	}
	if inAzure := in.Spec.Platform.Azure; inAzure != nil {
		if out.Spec.Platform.Azure == nil {
			out.Spec.Platform.Azure = &hiveapiazure.Platform{}
		}
		outAzure := out.Spec.Platform.Azure
		outAzure.Region = inAzure.Region
		outAzure.BaseDomainResourceGroupName = inAzure.BaseDomainResourceGroupName
		if out.Spec.PlatformSecrets.Azure == nil {
			out.Spec.PlatformSecrets.Azure = &hiveapiazure.PlatformSecrets{}
		}
		out.Spec.PlatformSecrets.Azure.Credentials = inAzure.CredentialsSecretRef
	} else {
		out.Spec.Platform.Azure = nil
		out.Spec.PlatformSecrets.Azure = nil
	}
	if inGCP := in.Spec.Platform.GCP; inGCP != nil {
		if out.Spec.Platform.GCP == nil {
			out.Spec.Platform.GCP = &hiveapigcp.Platform{}
		}
		out.Spec.Platform.GCP.Region = inGCP.Region
		if out.Spec.PlatformSecrets.GCP == nil {
			out.Spec.PlatformSecrets.GCP = &hiveapigcp.PlatformSecrets{}
		}
		out.Spec.PlatformSecrets.GCP.Credentials = inGCP.CredentialsSecretRef
	} else {
		out.Spec.Platform.GCP = nil
		out.Spec.PlatformSecrets.GCP = nil
	}
	out.Spec.PullSecret = in.Spec.PullSecretRef
	if p := in.Spec.Provisioning; p != nil {
		out.Spec.Images.ReleaseImage = p.ReleaseImage
		if imageSet := p.ImageSetRef; imageSet != nil {
			out.Spec.ImageSet = &hiveapi.ClusterImageSetReference{
				Name: imageSet.Name,
			}
		} else {
			out.Spec.ImageSet = nil
		}
	} else {
		out.Spec.Images.ReleaseImage = ""
		out.Spec.ImageSet = nil
	}
	out.Spec.PreserveOnDelete = in.Spec.PreserveOnDelete
	out.Spec.ControlPlaneConfig.ServingCertificates.Default = in.Spec.ControlPlaneConfig.ServingCertificates.Default
	if additional := in.Spec.ControlPlaneConfig.ServingCertificates.Additional; additional != nil {
		out.Spec.ControlPlaneConfig.ServingCertificates.Additional = make([]hiveapi.ControlPlaneAdditionalCertificate, len(additional))
		for i, inCert := range additional {
			outCert := &out.Spec.ControlPlaneConfig.ServingCertificates.Additional[i]
			outCert.Name = inCert.Name
			outCert.Domain = inCert.Domain
		}
	} else {
		out.Spec.ControlPlaneConfig.ServingCertificates.Additional = nil
	}
	if ingress := in.Spec.Ingress; ingress != nil {
		out.Spec.Ingress = make([]hiveapi.ClusterIngress, len(ingress))
		for i, inIngress := range ingress {
			outIngress := &out.Spec.Ingress[i]
			outIngress.Name = inIngress.Name
			outIngress.Domain = inIngress.Domain
			outIngress.NamespaceSelector = inIngress.NamespaceSelector
			outIngress.RouteSelector = inIngress.RouteSelector
			outIngress.ServingCertificate = inIngress.ServingCertificate
		}
	} else {
		out.Spec.Ingress = nil
	}
	if certs := in.Spec.CertificateBundles; certs != nil {
		out.Spec.CertificateBundles = make([]hiveapi.CertificateBundleSpec, len(certs))
		for i, inCert := range certs {
			outCert := &out.Spec.CertificateBundles[i]
			outCert.Name = inCert.Name
			outCert.Generate = inCert.Generate
			outCert.SecretRef = inCert.CertificateSecretRef
		}
	} else {
		out.Spec.CertificateBundles = nil
	}
	out.Spec.ManageDNS = in.Spec.ManageDNS
	out.Spec.Installed = in.Spec.Installed
	if meta := in.Spec.ClusterMetadata; meta != nil {
		out.Status.ClusterID = meta.ClusterID
		out.Status.InfraID = meta.InfraID
		out.Status.AdminKubeconfigSecret = meta.AdminKubeconfigSecretRef
		out.Status.AdminPasswordSecret = meta.AdminPasswordSecretRef
	} else {
		out.Status.ClusterID = ""
		out.Status.InfraID = ""
		out.Status.AdminKubeconfigSecret = corev1.LocalObjectReference{}
		out.Status.AdminPasswordSecret = corev1.LocalObjectReference{}
	}
	out.Status.Installed = in.Spec.Installed
	out.Status.InstallRestarts = in.Status.InstallRestarts
	out.Status.ClusterVersionStatus = in.Status.ClusterVersionStatus
	out.Status.APIURL = in.Status.APIURL
	out.Status.WebConsoleURL = in.Status.WebConsoleURL
	out.Status.InstallerImage = in.Status.InstallerImage
	out.Status.CLIImage = in.Status.CLIImage
	if conds := in.Status.Conditions; conds != nil {
		out.Status.Conditions = make([]hiveapi.ClusterDeploymentCondition, len(conds))
		for i, inCond := range conds {
			outCond := &out.Status.Conditions[i]
			outCond.Type = hiveapi.ClusterDeploymentConditionType(inCond.Type)
			outCond.Status = inCond.Status
			outCond.LastProbeTime = inCond.LastProbeTime
			outCond.LastTransitionTime = inCond.LastTransitionTime
			outCond.Reason = inCond.Reason
			outCond.Message = inCond.Message
		}
	} else {
		out.Status.Conditions = nil
	}
	if certs := in.Status.CertificateBundles; certs != nil {
		out.Status.CertificateBundles = make([]hiveapi.CertificateBundleStatus, len(certs))
		for i, inCert := range certs {
			outCert := &out.Status.CertificateBundles[i]
			outCert.Name = inCert.Name
			outCert.Generated = inCert.Generated
		}
	} else {
		out.Status.CertificateBundles = nil
	}
	out.Status.InstalledTimestamp = in.Status.InstalledTimestamp
	out.Status.Provision = in.Status.ProvisionRef
	return nil
}

func Convert_v1alpha1_ClusterImageSet_To_v1_ClusterImageSet(in *hiveapi.ClusterImageSet, out *hivev1.ClusterImageSet, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if in.Spec.ReleaseImage != nil {
		out.Spec.ReleaseImage = *in.Spec.ReleaseImage
	} else {
		out.Spec.ReleaseImage = ""
	}
	return nil
}

func Convert_v1_ClusterImageSet_To_v1alpha1_ClusterImageSet(in *hivev1.ClusterImageSet, out *hiveapi.ClusterImageSet, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ReleaseImage = pointer.StringPtr(in.Spec.ReleaseImage)
	return nil
}

func Convert_v1alpha1_ClusterDeprovisionRequest_To_v1_ClusterDeprovision(in *hiveapi.ClusterDeprovisionRequest, out *hivev1.ClusterDeprovision, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.InfraID = in.Spec.InfraID
	out.Spec.ClusterID = in.Spec.ClusterID
	if aws := in.Spec.Platform.AWS; aws != nil {
		if out.Spec.Platform.AWS == nil {
			out.Spec.Platform.AWS = &hivev1.AWSClusterDeprovision{}
		}
		out.Spec.Platform.AWS.Region = aws.Region
		out.Spec.Platform.AWS.CredentialsSecretRef = aws.Credentials
	} else {
		out.Spec.Platform.AWS = nil
	}
	if azure := in.Spec.Platform.Azure; azure != nil {
		if out.Spec.Platform.Azure == nil {
			out.Spec.Platform.Azure = &hivev1.AzureClusterDeprovision{}
		}
		out.Spec.Platform.Azure.CredentialsSecretRef = azure.Credentials
	} else {
		out.Spec.Platform.Azure = nil
	}
	if gcp := in.Spec.Platform.GCP; gcp != nil {
		if out.Spec.Platform.GCP == nil {
			out.Spec.Platform.GCP = &hivev1.GCPClusterDeprovision{}
		}
		out.Spec.Platform.GCP.Region = gcp.Region
		out.Spec.Platform.GCP.CredentialsSecretRef = gcp.Credentials
	} else {
		out.Spec.Platform.GCP = nil
	}
	out.Status.Completed = in.Status.Completed
	return nil
}

func Convert_v1_ClusterDeprovision_To_v1alpha1_ClusterDeprovisionRequest(in *hivev1.ClusterDeprovision, out *hiveapi.ClusterDeprovisionRequest, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.InfraID = in.Spec.InfraID
	out.Spec.ClusterID = in.Spec.ClusterID
	if aws := in.Spec.Platform.AWS; aws != nil {
		if out.Spec.Platform.AWS == nil {
			out.Spec.Platform.AWS = &hiveapi.AWSClusterDeprovisionRequest{}
		}
		out.Spec.Platform.AWS.Region = aws.Region
		out.Spec.Platform.AWS.Credentials = aws.CredentialsSecretRef
	} else {
		out.Spec.Platform.AWS = nil
	}
	if azure := in.Spec.Platform.Azure; azure != nil {
		if out.Spec.Platform.Azure == nil {
			out.Spec.Platform.Azure = &hiveapi.AzureClusterDeprovisionRequest{}
		}
		out.Spec.Platform.Azure.Credentials = azure.CredentialsSecretRef
	} else {
		out.Spec.Platform.Azure = nil
	}
	if gcp := in.Spec.Platform.GCP; gcp != nil {
		if out.Spec.Platform.GCP == nil {
			out.Spec.Platform.GCP = &hiveapi.GCPClusterDeprovisionRequest{}
		}
		out.Spec.Platform.GCP.Region = gcp.Region
		out.Spec.Platform.GCP.Credentials = gcp.CredentialsSecretRef
	}
	out.Status.Completed = in.Status.Completed
	return nil
}

func Convert_v1alpha1_ClusterProvision_To_v1_ClusterProvision(in *hiveapi.ClusterProvision, out *hivev1.ClusterProvision, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ClusterDeploymentRef = in.Spec.ClusterDeployment
	out.Spec.PodSpec = in.Spec.PodSpec
	out.Spec.Attempt = in.Spec.Attempt
	out.Spec.Stage = hivev1.ClusterProvisionStage(in.Spec.Stage)
	out.Spec.ClusterID = in.Spec.ClusterID
	out.Spec.InfraID = in.Spec.InfraID
	out.Spec.InstallLog = in.Spec.InstallLog
	out.Spec.Metadata = in.Spec.Metadata
	out.Spec.AdminKubeconfigSecretRef = in.Spec.AdminKubeconfigSecret
	out.Spec.AdminPasswordSecretRef = in.Spec.AdminPasswordSecret
	out.Spec.PrevClusterID = in.Spec.PrevClusterID
	out.Spec.PrevInfraID = in.Spec.PrevInfraID
	out.Status.JobRef = in.Status.Job
	if conds := in.Status.Conditions; conds != nil {
		out.Status.Conditions = make([]hivev1.ClusterProvisionCondition, len(conds))
		for i, inCond := range conds {
			outCond := &out.Status.Conditions[i]
			outCond.Type = hivev1.ClusterProvisionConditionType(inCond.Type)
			outCond.Status = inCond.Status
			outCond.LastProbeTime = inCond.LastProbeTime
			outCond.LastTransitionTime = inCond.LastTransitionTime
			outCond.Reason = inCond.Reason
			outCond.Message = inCond.Message
		}
	} else {
		out.Status.Conditions = nil
	}
	return nil
}

func Convert_v1_ClusterProvision_To_v1alpha1_ClusterProvision(in *hivev1.ClusterProvision, out *hiveapi.ClusterProvision, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ClusterDeployment = in.Spec.ClusterDeploymentRef
	out.Spec.PodSpec = in.Spec.PodSpec
	out.Spec.Attempt = in.Spec.Attempt
	out.Spec.Stage = hiveapi.ClusterProvisionStage(in.Spec.Stage)
	out.Spec.ClusterID = in.Spec.ClusterID
	out.Spec.InfraID = in.Spec.InfraID
	out.Spec.InstallLog = in.Spec.InstallLog
	out.Spec.Metadata = in.Spec.Metadata
	out.Spec.AdminKubeconfigSecret = in.Spec.AdminKubeconfigSecretRef
	out.Spec.AdminPasswordSecret = in.Spec.AdminPasswordSecretRef
	out.Spec.PrevClusterID = in.Spec.PrevClusterID
	out.Spec.PrevInfraID = in.Spec.PrevInfraID
	out.Status.Job = in.Status.JobRef
	if conds := in.Status.Conditions; conds != nil {
		out.Status.Conditions = make([]hiveapi.ClusterProvisionCondition, len(conds))
		for i, inCond := range conds {
			outCond := &out.Status.Conditions[i]
			outCond.Type = hiveapi.ClusterProvisionConditionType(inCond.Type)
			outCond.Status = inCond.Status
			outCond.LastProbeTime = inCond.LastProbeTime
			outCond.LastTransitionTime = inCond.LastTransitionTime
			outCond.Reason = inCond.Reason
			outCond.Message = inCond.Message
		}
	} else {
		out.Status.Conditions = nil
	}
	return nil
}

func Convert_v1alpha1_ClusterState_To_v1_ClusterState(in *hiveapi.ClusterState, out *hivev1.ClusterState, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Status.LastUpdated = in.Status.LastUpdated
	if ops := in.Status.ClusterOperators; ops != nil {
		out.Status.ClusterOperators = make([]hivev1.ClusterOperatorState, len(ops))
		for i, inOp := range ops {
			outOp := &out.Status.ClusterOperators[i]
			outOp.Name = inOp.Name
			outOp.Conditions = inOp.Conditions
		}
	} else {
		out.Status.ClusterOperators = nil
	}
	return nil
}

func Convert_v1_ClusterState_To_v1alpha1_ClusterState(in *hivev1.ClusterState, out *hiveapi.ClusterState, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Status.LastUpdated = in.Status.LastUpdated
	if ops := in.Status.ClusterOperators; ops != nil {
		out.Status.ClusterOperators = make([]hiveapi.ClusterOperatorState, len(ops))
		for i, inOp := range ops {
			outOp := &out.Status.ClusterOperators[i]
			outOp.Name = inOp.Name
			outOp.Conditions = inOp.Conditions
		}
	} else {
		out.Status.ClusterOperators = nil
	}
	return nil
}

func Convert_v1alpha1_DNSZone_To_v1_DNSZone(in *hiveapi.DNSZone, out *hivev1.DNSZone, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.Zone = in.Spec.Zone
	out.Spec.LinkToParentDomain = in.Spec.LinkToParentDomain
	if inAWS := in.Spec.AWS; inAWS != nil {
		if out.Spec.AWS == nil {
			out.Spec.AWS = &hivev1.AWSDNSZoneSpec{}
		}
		outAWS := out.Spec.AWS
		outAWS.CredentialsSecretRef = inAWS.AccountSecret
		if tags := inAWS.AdditionalTags; tags != nil {
			outAWS.AdditionalTags = make([]hivev1.AWSResourceTag, len(tags))
			for i, tag := range tags {
				outAWS.AdditionalTags[i] = hivev1.AWSResourceTag{
					Key:   tag.Key,
					Value: tag.Value,
				}
			}
		} else {
			outAWS.AdditionalTags = nil
		}
	} else {
		out.Spec.AWS = nil
	}
	if inGCP := in.Spec.GCP; inGCP != nil {
		if out.Spec.GCP == nil {
			out.Spec.GCP = &hivev1.GCPDNSZoneSpec{}
		}
		out.Spec.GCP.CredentialsSecretRef = inGCP.CredentialsSecretRef
	} else {
		out.Spec.GCP = nil
	}
	out.Status.LastSyncTimestamp = in.Status.LastSyncTimestamp
	out.Status.LastSyncGeneration = in.Status.LastSyncGeneration
	out.Status.NameServers = in.Status.NameServers
	if aws := in.Status.AWS; aws != nil {
		if out.Status.AWS == nil {
			out.Status.AWS = &hivev1.AWSDNSZoneStatus{}
		}
		out.Status.AWS.ZoneID = aws.ZoneID
	} else {
		out.Status.AWS = nil
	}
	if gcp := in.Status.GCP; gcp != nil {
		if out.Status.GCP == nil {
			out.Status.GCP = &hivev1.GCPDNSZoneStatus{}
		}
		out.Status.GCP.ZoneName = gcp.ZoneName
	} else {
		out.Status.GCP = nil
	}
	if conds := in.Status.Conditions; conds != nil {
		out.Status.Conditions = make([]hivev1.DNSZoneCondition, len(conds))
		for i, inCond := range conds {
			outCond := &out.Status.Conditions[i]
			outCond.Type = hivev1.DNSZoneConditionType(inCond.Type)
			outCond.Status = inCond.Status
			outCond.LastProbeTime = inCond.LastProbeTime
			outCond.LastTransitionTime = inCond.LastTransitionTime
			outCond.Reason = inCond.Reason
			outCond.Message = inCond.Message
		}
	} else {
		out.Status.Conditions = nil
	}
	return nil
}

func Convert_v1_DNSZone_To_v1alpha1_DNSZone(in *hivev1.DNSZone, out *hiveapi.DNSZone, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.Zone = in.Spec.Zone
	out.Spec.LinkToParentDomain = in.Spec.LinkToParentDomain
	if inAWS := in.Spec.AWS; inAWS != nil {
		if out.Spec.AWS == nil {
			out.Spec.AWS = &hiveapi.AWSDNSZoneSpec{}
		}
		outAWS := out.Spec.AWS
		outAWS.AccountSecret = inAWS.CredentialsSecretRef
		if tags := inAWS.AdditionalTags; tags != nil {
			outAWS.AdditionalTags = make([]hiveapi.AWSResourceTag, len(tags))
			for i, tag := range tags {
				outAWS.AdditionalTags[i] = hiveapi.AWSResourceTag{
					Key:   tag.Key,
					Value: tag.Value,
				}
			}
		} else {
			outAWS.AdditionalTags = nil
		}
	} else {
		out.Spec.AWS = nil
	}
	if inGCP := in.Spec.GCP; inGCP != nil {
		if out.Spec.GCP == nil {
			out.Spec.GCP = &hiveapi.GCPDNSZoneSpec{}
		}
		out.Spec.GCP.CredentialsSecretRef = inGCP.CredentialsSecretRef
	} else {
		out.Spec.GCP = nil
	}
	out.Status.LastSyncTimestamp = in.Status.LastSyncTimestamp
	out.Status.LastSyncGeneration = in.Status.LastSyncGeneration
	out.Status.NameServers = in.Status.NameServers
	if aws := in.Status.AWS; aws != nil {
		if out.Status.AWS == nil {
			out.Status.AWS = &hiveapi.AWSDNSZoneStatus{}
		}
		out.Status.AWS.ZoneID = aws.ZoneID
	} else {
		out.Status.AWS = nil
	}
	if gcp := in.Status.GCP; gcp != nil {
		if out.Status.GCP == nil {
			out.Status.GCP = &hiveapi.GCPDNSZoneStatus{}
		}
		out.Status.GCP.ZoneName = gcp.ZoneName
	} else {
		out.Status.GCP = nil
	}
	if conds := in.Status.Conditions; conds != nil {
		out.Status.Conditions = make([]hiveapi.DNSZoneCondition, len(conds))
		for i, inCond := range conds {
			outCond := &out.Status.Conditions[i]
			outCond.Type = hiveapi.DNSZoneConditionType(inCond.Type)
			outCond.Status = inCond.Status
			outCond.LastProbeTime = inCond.LastProbeTime
			outCond.LastTransitionTime = inCond.LastTransitionTime
			outCond.Reason = inCond.Reason
			outCond.Message = inCond.Message
		}
	} else {
		out.Status.Conditions = nil
	}
	return nil
}

func Convert_v1alpha1_HiveConfig_To_v1_HiveConfig(in *hiveapi.HiveConfig, out *hivev1.HiveConfig, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	if inDNS := in.Spec.ExternalDNS; inDNS != nil {
		out.Spec.ManagedDomains = []hivev1.ManageDNSConfig{{
			Domains: in.Spec.ManagedDomains,
		}}
		outDNS := &out.Spec.ManagedDomains[0]
		if aws := inDNS.AWS; aws != nil {
			outDNS.AWS = &hivev1.ManageDNSAWSConfig{
				CredentialsSecretRef: aws.Credentials,
			}
		}
		if gcp := inDNS.GCP; gcp != nil {
			outDNS.GCP = &hivev1.ManageDNSGCPConfig{
				CredentialsSecretRef: gcp.Credentials,
			}
		}
	} else {
		out.Spec.ManagedDomains = nil
	}
	out.Spec.AdditionalCertificateAuthoritiesSecretRef = in.Spec.AdditionalCertificateAuthorities
	out.Spec.GlobalPullSecretRef = in.Spec.GlobalPullSecret
	out.Spec.Backup.Velero.Enabled = in.Spec.Backup.Velero.Enabled
	out.Spec.Backup.MinBackupPeriodSeconds = in.Spec.Backup.MinBackupPeriodSeconds
	out.Spec.FailedProvisionConfig.SkipGatherLogs = in.Spec.FailedProvisionConfig.SkipGatherLogs
	out.Status.AggregatorClientCAHash = in.Status.AggregatorClientCAHash
	return nil
}

func Convert_v1_HiveConfig_To_v1alpha1_HiveConfig(in *hivev1.HiveConfig, out *hiveapi.HiveConfig, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	switch len(in.Spec.ManagedDomains) {
	case 0:
		out.Spec.ManagedDomains = nil
		out.Spec.ExternalDNS = nil
	case 1:
		inDNS := in.Spec.ManagedDomains[0]
		out.Spec.ManagedDomains = inDNS.Domains
		if out.Spec.ExternalDNS == nil {
			out.Spec.ExternalDNS = &hiveapi.ExternalDNSConfig{}
		}
		outDNS := out.Spec.ExternalDNS
		if aws := inDNS.AWS; aws != nil {
			if outDNS.AWS == nil {
				outDNS.AWS = &hiveapi.ExternalDNSAWSConfig{}
			}
			outDNS.AWS.Credentials = aws.CredentialsSecretRef
		} else {
			outDNS.AWS = nil
		}
		if gcp := inDNS.GCP; gcp != nil {
			if outDNS.GCP == nil {
				outDNS.GCP = &hiveapi.ExternalDNSGCPConfig{}
			}
			outDNS.GCP.Credentials = gcp.CredentialsSecretRef
		} else {
			outDNS.GCP = nil
		}
	default:
		return errors.New("cannot convert HiveConfig with multiple managed domains from v1 to v1alpha1")
	}
	out.Spec.AdditionalCertificateAuthorities = in.Spec.AdditionalCertificateAuthoritiesSecretRef
	out.Spec.GlobalPullSecret = in.Spec.GlobalPullSecretRef
	out.Spec.Backup.Velero.Enabled = in.Spec.Backup.Velero.Enabled
	out.Spec.Backup.MinBackupPeriodSeconds = in.Spec.Backup.MinBackupPeriodSeconds
	out.Spec.FailedProvisionConfig.SkipGatherLogs = in.Spec.FailedProvisionConfig.SkipGatherLogs
	out.Status.AggregatorClientCAHash = in.Status.AggregatorClientCAHash
	return nil
}

func convert_v1alpha1_SyncIdentityProviderCommonSpec_To_v1_SyncIdentityProviderCommonSpec(in *hiveapi.SyncIdentityProviderCommonSpec, out *hivev1.SyncIdentityProviderCommonSpec) {
	out.IdentityProviders = in.IdentityProviders
}

func convert_v1_SyncIdentityProviderCommonSpec_To_v1alpha1_SyncIdentityProviderCommonSpec(in *hivev1.SyncIdentityProviderCommonSpec, out *hiveapi.SyncIdentityProviderCommonSpec) {
	out.IdentityProviders = in.IdentityProviders
}

func Convert_v1alpha1_SyncIdentityProvider_To_v1_SyncIdentityProvider(in *hiveapi.SyncIdentityProvider, out *hivev1.SyncIdentityProvider, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	convert_v1alpha1_SyncIdentityProviderCommonSpec_To_v1_SyncIdentityProviderCommonSpec(&in.Spec.SyncIdentityProviderCommonSpec, &out.Spec.SyncIdentityProviderCommonSpec)
	out.Spec.ClusterDeploymentRefs = in.Spec.ClusterDeploymentRefs
	return nil
}

func Convert_v1_SyncIdentityProvider_To_v1alpha1_SyncIdentityProvider(in *hivev1.SyncIdentityProvider, out *hiveapi.SyncIdentityProvider, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	convert_v1_SyncIdentityProviderCommonSpec_To_v1alpha1_SyncIdentityProviderCommonSpec(&in.Spec.SyncIdentityProviderCommonSpec, &out.Spec.SyncIdentityProviderCommonSpec)
	out.Spec.ClusterDeploymentRefs = in.Spec.ClusterDeploymentRefs
	return nil
}

func Convert_v1alpha1_SelectorSyncIdentityProvider_To_v1_SelectorSyncIdentityProvider(in *hiveapi.SelectorSyncIdentityProvider, out *hivev1.SelectorSyncIdentityProvider, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	convert_v1alpha1_SyncIdentityProviderCommonSpec_To_v1_SyncIdentityProviderCommonSpec(&in.Spec.SyncIdentityProviderCommonSpec, &out.Spec.SyncIdentityProviderCommonSpec)
	out.Spec.ClusterDeploymentSelector = in.Spec.ClusterDeploymentSelector
	return nil
}

func Convert_v1_SelectorSyncIdentityProvider_To_v1alpha1_SelectorSyncIdentityProvider(in *hivev1.SelectorSyncIdentityProvider, out *hiveapi.SelectorSyncIdentityProvider, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	convert_v1_SyncIdentityProviderCommonSpec_To_v1alpha1_SyncIdentityProviderCommonSpec(&in.Spec.SyncIdentityProviderCommonSpec, &out.Spec.SyncIdentityProviderCommonSpec)
	out.Spec.ClusterDeploymentSelector = in.Spec.ClusterDeploymentSelector
	return nil
}

func convert_v1alpha1_SyncSetCommonSpec_To_v1_SyncSetCommonSpec(in *hiveapi.SyncSetCommonSpec, out *hivev1.SyncSetCommonSpec) {
	out.Resources = in.Resources
	out.ResourceApplyMode = hivev1.SyncSetResourceApplyMode(in.ResourceApplyMode)
	if patches := in.Patches; patches != nil {
		out.Patches = make([]hivev1.SyncObjectPatch, len(patches))
		for i, inPatch := range patches {
			outPatch := &out.Patches[i]
			outPatch.APIVersion = inPatch.APIVersion
			outPatch.Kind = inPatch.Kind
			outPatch.Name = inPatch.Name
			outPatch.Namespace = inPatch.Namespace
			outPatch.Patch = inPatch.Patch
			outPatch.PatchType = inPatch.PatchType
		}
	} else {
		out.Patches = nil
	}
	if secrets := in.SecretReferences; secrets != nil {
		out.Secrets = make([]hivev1.SecretMapping, len(secrets))
		for i, inRef := range secrets {
			outRef := &out.Secrets[i]
			outRef.SourceRef.Name = inRef.Source.Name
			outRef.SourceRef.Namespace = inRef.Source.Namespace
			outRef.TargetRef.Name = inRef.Target.Name
			outRef.TargetRef.Namespace = inRef.Target.Namespace
		}
	} else {
		out.Secrets = nil
	}
}

func convert_v1_SyncSetCommonSpec_To_v1alpha1_SyncSetCommonSpec(in *hivev1.SyncSetCommonSpec, out *hiveapi.SyncSetCommonSpec) {
	out.Resources = in.Resources
	out.ResourceApplyMode = hiveapi.SyncSetResourceApplyMode(in.ResourceApplyMode)
	if patches := in.Patches; patches != nil {
		out.Patches = make([]hiveapi.SyncObjectPatch, len(patches))
		for i, inPatch := range patches {
			outPatch := &out.Patches[i]
			outPatch.APIVersion = inPatch.APIVersion
			outPatch.Kind = inPatch.Kind
			outPatch.Name = inPatch.Name
			outPatch.Namespace = inPatch.Namespace
			outPatch.Patch = inPatch.Patch
			outPatch.PatchType = inPatch.PatchType
		}
	} else {
		out.Patches = nil
	}
	if secrets := in.Secrets; secrets != nil {
		out.SecretReferences = make([]hiveapi.SecretReference, len(secrets))
		for i, inRef := range secrets {
			outRef := &out.SecretReferences[i]
			outRef.Source.Name = inRef.SourceRef.Name
			outRef.Source.Namespace = inRef.SourceRef.Namespace
			outRef.Target.Name = inRef.TargetRef.Name
			outRef.Target.Namespace = inRef.TargetRef.Namespace
		}
	} else {
		out.SecretReferences = nil
	}
}

func Convert_v1alpha1_SyncSet_To_v1_SyncSet(in *hiveapi.SyncSet, out *hivev1.SyncSet, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	convert_v1alpha1_SyncSetCommonSpec_To_v1_SyncSetCommonSpec(&in.Spec.SyncSetCommonSpec, &out.Spec.SyncSetCommonSpec)
	out.Spec.ClusterDeploymentRefs = in.Spec.ClusterDeploymentRefs
	return nil
}

func Convert_v1_SyncSet_To_v1alpha1_SyncSet(in *hivev1.SyncSet, out *hiveapi.SyncSet, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	convert_v1_SyncSetCommonSpec_To_v1alpha1_SyncSetCommonSpec(&in.Spec.SyncSetCommonSpec, &out.Spec.SyncSetCommonSpec)
	out.Spec.ClusterDeploymentRefs = in.Spec.ClusterDeploymentRefs
	return nil
}

func Convert_v1alpha1_SelectorSyncSet_To_v1_SelectorSyncSet(in *hiveapi.SelectorSyncSet, out *hivev1.SelectorSyncSet, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	convert_v1alpha1_SyncSetCommonSpec_To_v1_SyncSetCommonSpec(&in.Spec.SyncSetCommonSpec, &out.Spec.SyncSetCommonSpec)
	out.Spec.ClusterDeploymentSelector = in.Spec.ClusterDeploymentSelector
	return nil
}

func Convert_v1_SelectorSyncSet_To_v1alpha1_SelectorSyncSet(in *hivev1.SelectorSyncSet, out *hiveapi.SelectorSyncSet, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	convert_v1_SyncSetCommonSpec_To_v1alpha1_SyncSetCommonSpec(&in.Spec.SyncSetCommonSpec, &out.Spec.SyncSetCommonSpec)
	out.Spec.ClusterDeploymentSelector = in.Spec.ClusterDeploymentSelector
	return nil
}

func convert_v1alpha1_SyncCondition_To_v1_SyncCondition(in *hiveapi.SyncCondition, out *hivev1.SyncCondition) {
	out.Type = hivev1.SyncConditionType(in.Type)
	out.Status = in.Status
	out.LastProbeTime = in.LastProbeTime
	out.LastTransitionTime = in.LastTransitionTime
	out.Reason = in.Reason
	out.Message = in.Message
}

func convert_v1_SyncCondition_To_v1alpha1_SyncCondition(in *hivev1.SyncCondition, out *hiveapi.SyncCondition) {
	out.Type = hiveapi.SyncConditionType(in.Type)
	out.Status = in.Status
	out.LastProbeTime = in.LastProbeTime
	out.LastTransitionTime = in.LastTransitionTime
	out.Reason = in.Reason
	out.Message = in.Message
}

func convert_v1alpha1_SyncConditionSlice_To_v1_SyncConditionSlice(in *[]hiveapi.SyncCondition, out *[]hivev1.SyncCondition) {
	if *in != nil {
		*out = make([]hivev1.SyncCondition, len(*in))
		for i, status := range *in {
			convert_v1alpha1_SyncCondition_To_v1_SyncCondition(&status, &(*out)[i])
		}
	} else {
		*out = nil
	}
}

func convert_v1_SyncConditionSlice_To_v1alpha1_SyncConditionSlice(in *[]hivev1.SyncCondition, out *[]hiveapi.SyncCondition) {
	if *in != nil {
		*out = make([]hiveapi.SyncCondition, len(*in))
		for i, status := range *in {
			convert_v1_SyncCondition_To_v1alpha1_SyncCondition(&status, &(*out)[i])
		}
	} else {
		*out = nil
	}
}

func convert_v1alpha1_SyncStatus_To_v1_SyncStatus(in *hiveapi.SyncStatus, out *hivev1.SyncStatus) {
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Resource = in.Resource
	out.Name = in.Name
	out.Namespace = in.Namespace
	out.Hash = in.Hash
	convert_v1alpha1_SyncConditionSlice_To_v1_SyncConditionSlice(&in.Conditions, &out.Conditions)
}

func convert_v1_SyncStatus_To_v1alpha1_SyncStatus(in *hivev1.SyncStatus, out *hiveapi.SyncStatus) {
	out.APIVersion = in.APIVersion
	out.Kind = in.Kind
	out.Resource = in.Resource
	out.Name = in.Name
	out.Namespace = in.Namespace
	out.Hash = in.Hash
	convert_v1_SyncConditionSlice_To_v1alpha1_SyncConditionSlice(&in.Conditions, &out.Conditions)
}

func convert_v1alpha1_SyncStatusSlice_To_v1_SyncStatusSlice(in *[]hiveapi.SyncStatus, out *[]hivev1.SyncStatus) {
	if *in != nil {
		*out = make([]hivev1.SyncStatus, len(*in))
		for i, status := range *in {
			convert_v1alpha1_SyncStatus_To_v1_SyncStatus(&status, &(*out)[i])
		}
	} else {
		*out = nil
	}
}

func convert_v1_SyncStatusSlice_To_v1alpha1_SyncStatusSlice(in *[]hivev1.SyncStatus, out *[]hiveapi.SyncStatus) {
	if *in != nil {
		*out = make([]hiveapi.SyncStatus, len(*in))
		for i, status := range *in {
			convert_v1_SyncStatus_To_v1alpha1_SyncStatus(&status, &(*out)[i])
		}
	} else {
		*out = nil
	}
}

func Convert_v1alpha1_SyncSetInstance_To_v1_SyncSetInstance(in *hiveapi.SyncSetInstance, out *hivev1.SyncSetInstance, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ClusterDeploymentRef = in.Spec.ClusterDeployment
	out.Spec.SyncSetRef = in.Spec.SyncSet
	if ref := in.Spec.SelectorSyncSet; ref != nil {
		out.Spec.SelectorSyncSetRef = &hivev1.SelectorSyncSetReference{
			Name: ref.Name,
		}
	} else {
		out.Spec.SelectorSyncSetRef = nil
	}
	out.Spec.ResourceApplyMode = hivev1.SyncSetResourceApplyMode(in.Spec.ResourceApplyMode)
	out.Spec.SyncSetHash = in.Spec.SyncSetHash
	convert_v1alpha1_SyncStatusSlice_To_v1_SyncStatusSlice(&in.Status.Resources, &out.Status.Resources)
	convert_v1alpha1_SyncStatusSlice_To_v1_SyncStatusSlice(&in.Status.Patches, &out.Status.Patches)
	convert_v1alpha1_SyncStatusSlice_To_v1_SyncStatusSlice(&in.Status.SecretReferences, &out.Status.Secrets)
	convert_v1alpha1_SyncConditionSlice_To_v1_SyncConditionSlice(&in.Status.Conditions, &out.Status.Conditions)
	return nil
}

func Convert_v1_SyncSetInstance_To_v1alpha1_SyncSetInstance(in *hivev1.SyncSetInstance, out *hiveapi.SyncSetInstance, _ conversion.Scope) error {
	out.ObjectMeta = in.ObjectMeta
	out.Spec.ClusterDeployment = in.Spec.ClusterDeploymentRef
	out.Spec.SyncSet = in.Spec.SyncSetRef
	if ref := in.Spec.SelectorSyncSetRef; ref != nil {
		out.Spec.SelectorSyncSet = &hiveapi.SelectorSyncSetReference{
			Name: ref.Name,
		}
	} else {
		out.Spec.SelectorSyncSet = nil
	}
	out.Spec.ResourceApplyMode = hiveapi.SyncSetResourceApplyMode(in.Spec.ResourceApplyMode)
	out.Spec.SyncSetHash = in.Spec.SyncSetHash
	convert_v1_SyncStatusSlice_To_v1alpha1_SyncStatusSlice(&in.Status.Resources, &out.Status.Resources)
	convert_v1_SyncStatusSlice_To_v1alpha1_SyncStatusSlice(&in.Status.Patches, &out.Status.Patches)
	convert_v1_SyncStatusSlice_To_v1alpha1_SyncStatusSlice(&in.Status.Secrets, &out.Status.SecretReferences)
	convert_v1_SyncConditionSlice_To_v1alpha1_SyncConditionSlice(&in.Status.Conditions, &out.Status.Conditions)
	return nil
}
