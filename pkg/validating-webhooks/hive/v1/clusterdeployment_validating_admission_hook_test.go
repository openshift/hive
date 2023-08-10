package v1

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	hivev1agent "github.com/openshift/hive/apis/hive/v1/agent"
	hivev1alibabacloud "github.com/openshift/hive/apis/hive/v1/alibabacloud"
	hivev1aws "github.com/openshift/hive/apis/hive/v1/aws"
	hivev1azure "github.com/openshift/hive/apis/hive/v1/azure"
	hivev1gcp "github.com/openshift/hive/apis/hive/v1/gcp"
	hivev1ibmcloud "github.com/openshift/hive/apis/hive/v1/ibmcloud"
	hivev1openstack "github.com/openshift/hive/apis/hive/v1/openstack"
	hivev1ovirt "github.com/openshift/hive/apis/hive/v1/ovirt"
	hivev1vsphere "github.com/openshift/hive/apis/hive/v1/vsphere"
	hivecontractsv1alpha1 "github.com/openshift/hive/apis/hivecontracts/v1alpha1"

	"github.com/openshift/hive/pkg/constants"
	"github.com/openshift/hive/pkg/util/contracts"
)

var validTestManagedDomains = []string{
	"aaa.com",
	"foo.aaa.com",
	"bbb.com",
	"ccc.com",
}

func clusterDeploymentTemplate() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		// TODO: Remove TypeMeta field once https://github.com/kubernetes-sigs/controller-runtime/issues/2429 is fixed
		TypeMeta: metav1.TypeMeta{
			APIVersion: hivev1.SchemeGroupVersion.String(),
			Kind:       "ClusterDeployment",
		},
		Spec: hivev1.ClusterDeploymentSpec{
			BaseDomain:  "example.com",
			ClusterName: "SameClusterName",
			Provisioning: &hivev1.Provisioning{
				InstallConfigSecretRef: &corev1.LocalObjectReference{
					Name: "test-install-config",
				},
			},
		},
	}
}
func validClusterDeploymentWithIngress() *hivev1.ClusterDeployment {
	cd := validAWSClusterDeployment()
	cd.Spec.Ingress = []hivev1.ClusterIngress{
		{
			Name:   "default",
			Domain: "apps.sameclustername.example.com",
		},
	}
	return cd
}

func clusterDeploymentWithManagedDomain(domain string) *hivev1.ClusterDeployment {
	cd := validAWSClusterDeployment()
	cd.Spec.ManageDNS = true
	cd.Spec.BaseDomain = domain
	return cd
}

func validGCPClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.GCP = &hivev1gcp.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:               "us-central1",
	}
	return cd
}

func validAlibabaCloudClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.AlibabaCloud = &hivev1alibabacloud.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:               "test-alibaba-cloud-region",
	}
	cd.Spec.Provisioning.ManifestsConfigMapRef = &corev1.LocalObjectReference{Name: "fake-manifests-configmap"}
	return cd
}

func validAWSClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.AWS = &hivev1aws.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:               "test-region",
	}
	return cd
}

func validAWSClusterDeploymentFromPool(poolNS, poolName, claimName string) *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.AWS = &hivev1aws.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:               "test-region",
	}
	cd.Spec.ClusterPoolRef = &hivev1.ClusterPoolReference{
		Namespace: poolNS,
		PoolName:  poolName,
		ClaimName: claimName,
	}
	return cd
}

func validAzureClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.Azure = &hivev1azure.Platform{
		CredentialsSecretRef:        corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:                      "test-region",
		BaseDomainResourceGroupName: "os4-common",
	}
	return cd
}

func validOpenStackClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.OpenStack = &hivev1openstack.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Cloud:                "somecloud",
	}
	return cd
}

func validVSphereClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.VSphere = &hivev1vsphere.Platform{
		VCenter:               "somevcenter.com",
		CredentialsSecretRef:  corev1.LocalObjectReference{Name: "fake-creds-secret"},
		CertificatesSecretRef: corev1.LocalObjectReference{Name: "fake-cert-secret"},
		Datacenter:            "dc1",
		DefaultDatastore:      "vmse-test",
		Folder:                "/dc1/vm/test",
		Cluster:               "test",
		Network:               "Network",
	}
	return cd
}

func validOvirtClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.Ovirt = &hivev1ovirt.Platform{
		ClusterID:             "fake-cluster-uuid",
		CredentialsSecretRef:  corev1.LocalObjectReference{Name: "fake-creds-secret"},
		CertificatesSecretRef: corev1.LocalObjectReference{Name: "fake-cert-secret"},
		StorageDomainID:       "fake-storage-domain-uuid",
	}
	return cd
}

func validIBMCloudClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.IBMCloud = &hivev1ibmcloud.Platform{
		CredentialsSecretRef: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		Region:               "us-east",
	}
	cd.Spec.Provisioning.ManifestsSecretRef = &corev1.LocalObjectReference{Name: "fake-manifests-secret"}
	return cd
}

func validAgentBareMetalClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform.AgentBareMetal = &hivev1agent.BareMetalPlatform{
		AgentSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				"foo": "bar",
			},
		},
	}
	cd.Spec.Provisioning.InstallConfigSecretRef = nil
	return cd
}

// Meant to be used to compare new and old as the same values.
func validClusterDeploymentSameValues() *hivev1.ClusterDeployment {
	return validAWSClusterDeployment()
}

func validClusterDeploymentDifferentImmutableValue() *hivev1.ClusterDeployment {
	cd := validAWSClusterDeployment()
	cd.Spec.ClusterName = "DifferentClusterName"
	cd.Spec.BaseDomain = "DifferenceBaseDomain"
	return cd
}

func validClusterDeploymentDifferentMutableValue() *hivev1.ClusterDeployment {
	cd := validAWSClusterDeployment()
	cd.Spec.PreserveOnDelete = true
	return cd
}

func TestClusterDeploymentValidatingResource(t *testing.T) {
	// Arrange
	data := NewClusterDeploymentValidatingAdmissionHook(createDecoder(t))
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1",
		Resource: "clusterdeploymentvalidators",
	}
	expectedSingular := "clusterdeploymentvalidator"

	// Act
	plural, singular := data.ValidatingResource()

	// Assert
	assert.Equal(t, expectedPlural, plural)
	assert.Equal(t, expectedSingular, singular)
}

func TestClusterDeploymentInitialize(t *testing.T) {
	// Arrange
	data := NewClusterDeploymentValidatingAdmissionHook(createDecoder(t))

	// Act
	err := data.Initialize(nil, nil)

	// Assert
	assert.Nil(t, err)
}

func TestClusterDeploymentValidate(t *testing.T) {
	cases := []struct {
		name                string
		newObject           *hivev1.ClusterDeployment
		newObjectRaw        []byte
		oldObject           *hivev1.ClusterDeployment
		oldObjectRaw        []byte
		operation           admissionv1beta1.Operation
		expectedAllowed     bool
		gvr                 *metav1.GroupVersionResource
		enabledFeatureGates []string
		awsPrivateLink      *hivev1.AWSPrivateLinkConfig
		supportedContracts  contracts.SupportedContractImplementationsList
	}{
		{
			name:            "Test valid create",
			newObject:       validAWSClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "Test Delete Operation is allowed even with mismatch objects",
			oldObject:       validAWSClusterDeployment(),
			newObject:       validClusterDeploymentDifferentImmutableValue(),
			operation:       admissionv1beta1.Delete,
			expectedAllowed: true,
		},
		{
			name:            "Test Update Operation is allowed with same data",
			oldObject:       validAWSClusterDeployment(),
			newObject:       validClusterDeploymentSameValues(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:            "Test Update Operation is allowed with different mutable data",
			oldObject:       validAWSClusterDeployment(),
			newObject:       validClusterDeploymentDifferentMutableValue(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:      "Test reject missing InstallConfigSecretRef",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Provisioning.InstallConfigSecretRef = nil
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:      "Test setting installed flag",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name: "Test clearing installed flag",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Installed = true
				return cd
			}(),
			newObject:       validAWSClusterDeployment(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test create with ClusterPoolReference",
			newObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", ""),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "Test create with Claimed ClusterPoolReference",
			newObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", "test-claim"),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Test DR create (restore) with Claimed ClusterPoolReference",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeploymentFromPool("pool-ns", "mypool", "test-claim")
				cd.Labels = map[string]string{constants.DisableCreationWebHookForDisasterRecovery: "true"}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "Test update with removed ClusterPoolReference",
			oldObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", ""),
			newObject:       validAWSClusterDeployment(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test update with added ClusterPoolReference",
			oldObject:       validAWSClusterDeployment(),
			newObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", ""),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test update with modified ClusterPoolReference",
			oldObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", ""),
			newObject:       validAWSClusterDeploymentFromPool("new-pool-ns", "new-mypool", ""),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test update with claimed ClusterPoolReference",
			oldObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", ""),
			newObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", "test-claim"),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:            "Test update with unclaimed ClusterPoolReference",
			oldObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", "test-claim"),
			newObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", ""),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test update with changed claim",
			oldObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", "test-claim"),
			newObject:       validAWSClusterDeploymentFromPool("pool-ns", "mypool", "other-claim"),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:      "Test setting installed flag without cluster metadata",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Installed = true
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name: "Test mutating cluster metadata after installed",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "old-infra-id",
				}
				return cd
			}(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "new-infra-id",
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:      "Test Update PreserveOnDelete",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.PreserveOnDelete = true
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:            "Test Update Operation is NOT allowed with different immutable data",
			oldObject:       validAWSClusterDeployment(),
			newObject:       validClusterDeploymentDifferentImmutableValue(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test unable to marshal new object during create",
			newObjectRaw:    []byte{0},
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "Test unable to marshal new object during update",
			newObjectRaw:    []byte{0},
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test unable to marshal old object during update",
			oldObjectRaw:    []byte{0},
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name: "Test doesn't validate with right version and resource, but wrong group",
			gvr: &metav1.GroupVersionResource{
				Group:    "not the right group",
				Version:  "v1",
				Resource: "clusterdeployments",
			},
			expectedAllowed: true,
		},
		{
			name: "Test doesn't validate with right group and resource, wrong version",
			gvr: &metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "not the right version",
				Resource: "clusterdeployments",
			},
			expectedAllowed: true,
		},
		{
			name: "Test doesn't validate with right group and version, wrong resource",
			gvr: &metav1.GroupVersionResource{
				Group:    "hive.openshift.io",
				Version:  "v1",
				Resource: "not the right resource",
			},
			expectedAllowed: true,
		},

		{
			name:            "Test going from previously defined list of ingress to empty ingress list",
			oldObject:       validClusterDeploymentWithIngress(),
			newObject:       validAWSClusterDeployment(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test new clusterdeployment with ingress list with default defined",
			newObject:       validClusterDeploymentWithIngress(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "Test new clusterdeployment with ingress list without default defined",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validClusterDeploymentWithIngress()
				cd.Spec.Ingress[0].Name = "notdefault"
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Test new clusterdeployment with missing SSH private key name",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validClusterDeploymentWithIngress()
				cd.Spec.Provisioning.SSHPrivateKeySecretRef = &corev1.LocalObjectReference{Name: ""}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "Test updating existing empty ingress to populated ingress",
			oldObject:       validAWSClusterDeployment(),
			newObject:       validClusterDeploymentWithIngress(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:      "Test updating existing ingress to one missing default",
			oldObject: validClusterDeploymentWithIngress(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validClusterDeploymentWithIngress()
				cd.Spec.Ingress[0].Name = "notdefault"
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:            "Test valid managed domain",
			newObject:       clusterDeploymentWithManagedDomain("bar.foo.aaa.com"),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "Test base domain is not child of a managed domain",
			newObject:       clusterDeploymentWithManagedDomain("bar.bad-domain.com"),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "Test base domain is not direct child of a managed domain",
			newObject:       clusterDeploymentWithManagedDomain("baz.foo.bbb.com"),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "Test base domain is not same as one of the managed domains",
			newObject:       clusterDeploymentWithManagedDomain("foo.aaa.com"),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Test managed DNS is valid on GCP",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validGCPClusterDeployment()
				cd.Spec.ManageDNS = true
				cd.Spec.BaseDomain = "bar.foo.aaa.com"
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "Test managed DNS is valid on Azure",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.ManageDNS = true
				cd.Spec.BaseDomain = "bar.foo.aaa.com"
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:      "Test allow modifying controlPlaneConfig",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.ControlPlaneConfig = hivev1.ControlPlaneConfigSpec{
					ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
						Default: "someNonExistentCertificateBundle",
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name:      "Test allow modifying certificateBundles",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
					{
						Name:     "testCertificateBundle",
						Generate: false,
						CertificateSecretRef: corev1.LocalObjectReference{
							Name: "testCertBundle-Secret",
						},
					},
				}

				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name: "Test allow modifying installAttemptsLimit",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.InstallAttemptsLimit = new(int32) // zero
				return cd
			}(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.InstallAttemptsLimit = new(int32) // zero
				*cd.Spec.InstallAttemptsLimit = 1
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name: "Test invalid wildcard ingress domain",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validClusterDeploymentWithIngress()
				cd.Spec.Ingress[0].Domain = "*.apps.sameclustername.example.com"
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Test invalid domain on ingress domain",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validClusterDeploymentWithIngress()
				cd.Spec.Ingress[0].Domain = "apps.sameclustername.NOTexample.com"
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Cluster deployment name is too long",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Name = "this-is-a-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-name"
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Cluster name is too long",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.ClusterName = "this-is-a-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-long-name"
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "Azure create valid",
			newObject:       validAzureClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "Azure create missing region",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Platform.Azure.Region = ""
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Azure create missing credentials",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Platform.Azure.CredentialsSecretRef.Name = ""
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "Azure create missing baseDomainResourceGroupName",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Platform.Azure.BaseDomainResourceGroupName = ""
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:      "Azure update region",
			oldObject: validAzureClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Platform.Azure.Region = ""
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name: "Azure set resource group name: installed",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
				}
				return cd
			}(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
					Platform: &hivev1.ClusterPlatformMetadata{
						Azure: &hivev1azure.Metadata{
							ResourceGroupName: pointer.String("my-rg"),
						},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name: "Azure set resource group name: not installed",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
				}
				return cd
			}(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
					Platform: &hivev1.ClusterPlatformMetadata{
						Azure: &hivev1azure.Metadata{
							ResourceGroupName: pointer.String("my-rg"),
						},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name: "Azure set resource group name: while setting installed",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
				}
				return cd
			}(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
					Platform: &hivev1.ClusterPlatformMetadata{
						Azure: &hivev1azure.Metadata{
							ResourceGroupName: pointer.String("my-rg"),
						},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name: "Azure update resource group name",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
				}
				return cd
			}(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
					Platform: &hivev1.ClusterPlatformMetadata{
						Azure: &hivev1azure.Metadata{
							ResourceGroupName: pointer.String("my-rg"),
						},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name: "Azure unset resource group name",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					InfraID: "an-infra-id",
				}
				return cd
			}(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Installed = true
				cd.Spec.ClusterMetadata = &hivev1.ClusterMetadata{
					Platform: &hivev1.ClusterPlatformMetadata{
						Azure: &hivev1azure.Metadata{
							ResourceGroupName: pointer.String("my-rg"),
						},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		// TODO: ensure Azure clusterDeployments have necessary info for
		// machine sets
		// {
		// 	name: "not setting instance type on Azure machine pool",
		// 	newObject: func() *hivev1.ClusterDeployment {
		// 		cd := validAzureClusterDeployment()
		// 		cd.Spec.Compute = []hivev1.MachinePool{
		// 			{
		// 				Name: "testmachinepool",
		// 				Platform: hivev1.MachinePoolPlatform{
		// 					Azure: &hivev1azure.MachinePool{
		// 						InstanceType: "",
		// 					},
		// 				},
		// 			},
		// 		}
		// 		return cd
		// 	}(),
		// 	operation:       admissionv1beta1.Create,
		// 	expectedAllowed: false,
		// },
		{
			name: "create with two cloud platforms",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				awsCD := validAWSClusterDeployment()
				cd.Spec.Platform.AWS = awsCD.Spec.Platform.AWS
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "create with no cloud platforms",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.Platform.Azure = nil
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "ingress with serving certificate",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validClusterDeploymentWithIngress()
				cd.Spec.Ingress[0].ServingCertificate = "test-serving-cert"
				cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
					{
						Name:                 "test-serving-cert",
						CertificateSecretRef: corev1.LocalObjectReference{Name: "test-serving-cert-secret"},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "ingress with missing serving certificate",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validClusterDeploymentWithIngress()
				cd.Spec.Ingress[0].ServingCertificate = "missing-serving-cert"
				cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
					{
						Name:                 "test-serving-cert",
						CertificateSecretRef: corev1.LocalObjectReference{Name: "test-serving-cert-secret"},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "valid serving certificate",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
					{
						Name:                 "test-serving-cert",
						CertificateSecretRef: corev1.LocalObjectReference{Name: "test-serving-cert-secret"},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "serving certificate without name",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
					{
						CertificateSecretRef: corev1.LocalObjectReference{Name: "test-serving-cert-secret"},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "serving certificate without secret reference",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.CertificateBundles = []hivev1.CertificateBundleSpec{
					{
						Name: "test-serving-cert",
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "InstallConfig is missing",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Provisioning.InstallConfigSecretRef = nil
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "ClusterInstallRef is set with Provisioning",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.ClusterInstallRef = &hivev1.ClusterInstallLocalReference{
					Group:   "hive.openshift.io",
					Version: "v1",
					Kind:    "FakeClusterInstall",
					Name:    "dummy",
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "ClusterInstallRef is set when no supported",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Provisioning = nil
				cd.Spec.ClusterInstallRef = &hivev1.ClusterInstallLocalReference{
					Group:   "hive.openshift.io",
					Version: "v1",
					Kind:    "FakeClusterInstall",
					Name:    "dummy",
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "ClusterInstallRef is set when not supported",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Provisioning = nil
				cd.Spec.ClusterInstallRef = &hivev1.ClusterInstallLocalReference{
					Group:   "hive.openshift.io",
					Version: "v1",
					Kind:    "FakeClusterInstall",
					Name:    "dummy",
				}
				return cd
			}(),
			supportedContracts: contracts.SupportedContractImplementationsList{{
				Name: hivecontractsv1alpha1.ClusterInstallContractName,
				Supported: []contracts.ContractImplementation{{
					Group:   "hive.openshift.io",
					Version: "v1",
					Kind:    "AgentClusterInstall",
				}},
			}},
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "ClusterInstallRef is set when supported",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Provisioning = nil
				cd.Spec.ClusterInstallRef = &hivev1.ClusterInstallLocalReference{
					Group:   "hive.openshift.io",
					Version: "v1",
					Kind:    "FakeClusterInstall",
					Name:    "dummy",
				}
				return cd
			}(),
			supportedContracts: contracts.SupportedContractImplementationsList{{
				Name: hivecontractsv1alpha1.ClusterInstallContractName,
				Supported: []contracts.ContractImplementation{{
					Group:   "hive.openshift.io",
					Version: "v1",
					Kind:    "FakeClusterInstall",
				}},
			}},
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "ClusterInstallRef and Provisioning both not set",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Provisioning = nil
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "valid GCP clusterdeployment",
			newObject:       validGCPClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "Provisioning is missing",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Provisioning = nil
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "OpenStack create valid",
			newObject:       validOpenStackClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "OpenStack create without secret reference for certificates",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validOpenStackClusterDeployment()
				cd.Spec.Platform.OpenStack.CertificatesSecretRef = &corev1.LocalObjectReference{Name: ""}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "OpenStack create with valid secret reference for certificates",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validOpenStackClusterDeployment()
				cd.Spec.Platform.OpenStack.CertificatesSecretRef = &corev1.LocalObjectReference{Name: "openstack-certificates"}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "Test valid delete",
			oldObject:       validAWSClusterDeployment(),
			operation:       admissionv1beta1.Delete,
			expectedAllowed: true,
		},
		{
			name: "Test protected delete",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				if cd.Annotations == nil {
					cd.Annotations = make(map[string]string, 1)
				}
				cd.Annotations[constants.ProtectedDeleteAnnotation] = "true"
				return cd
			}(),
			operation:       admissionv1beta1.Delete,
			expectedAllowed: false,
		},
		{
			name: "Test protected delete annotation false",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				if cd.Annotations == nil {
					cd.Annotations = make(map[string]string, 1)
				}
				cd.Annotations[constants.ProtectedDeleteAnnotation] = "false"
				return cd
			}(),
			operation:       admissionv1beta1.Delete,
			expectedAllowed: true,
		},
		{
			name:            "vSphere create valid",
			newObject:       validVSphereClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "oVirt create valid",
			newObject:       validOvirtClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name:            "IBMCloud create valid",
			newObject:       validIBMCloudClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "IBMCloud create missing manifests",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validIBMCloudClusterDeployment()
				cd.Spec.Provisioning.ManifestsConfigMapRef = nil
				cd.Spec.Provisioning.ManifestsSecretRef = nil
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name:            "Alibaba Cloud create valid",
			newObject:       validAlibabaCloudClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "Alibaba Cloud create missing manifests",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAlibabaCloudClusterDeployment()
				cd.Spec.Provisioning.ManifestsConfigMapRef = nil
				cd.Spec.Provisioning.ManifestsSecretRef = nil
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "manifestsConfigMapRef and manifestsSecretRef mutually exclusive (create)",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Provisioning.ManifestsConfigMapRef = &corev1.LocalObjectReference{
					Name: "foo",
				}
				cd.Spec.Provisioning.ManifestsSecretRef = &corev1.LocalObjectReference{Name: "bar"}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			// This gets caught by the "Spec is immutable except [... not Provisioning ...]" check
			name:      "manifestsConfigMapRef and manifestsSecretRef mutually exclusive (update)",
			oldObject: validAlibabaCloudClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAlibabaCloudClusterDeployment()
				// Already has ManifestsConfigMapRef
				cd.Spec.Provisioning.ManifestsSecretRef = &corev1.LocalObjectReference{Name: "bar"}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name: "private link set, but disabled, no config",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccess{}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "private link enabled, no config",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccess{Enabled: true}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "private link enabled, no inventory",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccess{Enabled: true}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
			awsPrivateLink:  &hivev1.AWSPrivateLinkConfig{},
		},
		{
			name: "private link enabled, no inventory in the given region",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccess{Enabled: true}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
			awsPrivateLink: &hivev1.AWSPrivateLinkConfig{
				EndpointVPCInventory: []hivev1.AWSPrivateLinkInventory{{
					AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
						Region: "some-region",
						VPCID:  "vpc-id",
					},
				}},
			},
		},
		{
			name: "private link enabled, some inventory in given region",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccess{Enabled: true}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
			awsPrivateLink: &hivev1.AWSPrivateLinkConfig{
				EndpointVPCInventory: []hivev1.AWSPrivateLinkInventory{{
					AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
						Region: "some-region",
						VPCID:  "vpc-id",
					},
				}, {
					AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
						Region: "test-region",
						VPCID:  "vpc-id-2",
					},
				}},
			},
		},
		{
			name:      "cd.spec.platform.agentBareMetal.agentSelector is a mutable field",
			oldObject: validAgentBareMetalClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAgentBareMetalClusterDeployment()
				// use different label selector
				cd.Spec.Platform.AgentBareMetal.AgentSelector = metav1.LabelSelector{
					MatchLabels: map[string]string{
						"bar": "baz",
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
		},
		{
			name: "cd.spec.platform.aws.privateLink.additionalAllowedPrincipals is a mutable field",
			oldObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccess{Enabled: true}
				return cd
			}(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Platform.AWS.PrivateLink = &hivev1aws.PrivateLinkAccess{
					Enabled:                     true,
					AdditionalAllowedPrincipals: &[]string{"aws:iam:12345:some-user"},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: true,
			awsPrivateLink: &hivev1.AWSPrivateLinkConfig{
				EndpointVPCInventory: []hivev1.AWSPrivateLinkInventory{{
					AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
						Region: "some-region",
						VPCID:  "vpc-id",
					},
				}, {
					AWSPrivateLinkVPC: hivev1.AWSPrivateLinkVPC{
						Region: "test-region",
						VPCID:  "vpc-id-2",
					},
				}},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			data := ClusterDeploymentValidatingAdmissionHook{
				decoder:             createDecoder(t),
				validManagedDomains: validTestManagedDomains,
				fs: &featureSet{
					FeatureGatesEnabled: &hivev1.FeatureGatesEnabled{
						Enabled: tc.enabledFeatureGates,
					},
				},
				awsPrivateLinkConfig: tc.awsPrivateLink,
				supportedContracts:   tc.supportedContracts,
			}

			if tc.gvr == nil {
				tc.gvr = &metav1.GroupVersionResource{
					Group:    "hive.openshift.io",
					Version:  "v1",
					Resource: "clusterdeployments",
				}
			}

			if tc.newObjectRaw == nil {
				tc.newObjectRaw, _ = json.Marshal(tc.newObject)
			}

			if tc.oldObjectRaw == nil {
				tc.oldObjectRaw, _ = json.Marshal(tc.oldObject)
			}

			request := &admissionv1beta1.AdmissionRequest{
				Operation: tc.operation,
				Resource:  *tc.gvr,
				Object: runtime.RawExtension{
					Raw: tc.newObjectRaw,
				},
				OldObject: runtime.RawExtension{
					Raw: tc.oldObjectRaw,
				},
			}

			// Act
			response := data.Validate(request)

			// Assert
			if !assert.Equal(t, tc.expectedAllowed, response.Allowed) {
				t.Logf("Response result = %#v", response.Result)
			}
		})
	}
}

func TestNewClusterDeploymentValidatingAdmissionHook(t *testing.T) {
	tempFile, err := os.CreateTemp("", "")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	domains := []hivev1.ManageDNSConfig{
		{
			Domains: []string{
				"first.domain.com",
				"second.domain.com",
				"third.domain.com",
			},
		},
		{
			Domains: []string{
				"extra.domain.com",
			},
		},
	}

	expectedDomains := []string{
		"first.domain.com",
		"second.domain.com",
		"third.domain.com",
		"extra.domain.com",
	}

	domainsJSON, err := json.Marshal(domains)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	_, err = tempFile.Write(domainsJSON)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	err = tempFile.Close()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	os.Setenv(constants.ManagedDomainsFileEnvVar, tempFile.Name())
	webhook := NewClusterDeploymentValidatingAdmissionHook(createDecoder(t))
	assert.Equal(t, webhook.validManagedDomains, expectedDomains, "valid domains must match expected")
}
