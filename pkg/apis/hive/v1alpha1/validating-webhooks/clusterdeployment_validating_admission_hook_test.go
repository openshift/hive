package validatingwebhooks

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	admissionv1beta1 "k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	hivev1aws "github.com/openshift/hive/pkg/apis/hive/v1alpha1/aws"
	hivev1azure "github.com/openshift/hive/pkg/apis/hive/v1alpha1/azure"
	hivev1gcp "github.com/openshift/hive/pkg/apis/hive/v1alpha1/gcp"
	"github.com/openshift/hive/pkg/constants"
)

const (
	defaultWorkerPoolName = "worker"
)

var validTestManagedDomains = []string{
	"aaa.com",
	"foo.aaa.com",
	"bbb.com",
	"ccc.com",
}

func clusterDeploymentTemplate() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			BaseDomain:  "example.com",
			ClusterName: "SameClusterName",
			Compute: []hivev1.MachinePool{
				{
					Name: defaultWorkerPoolName,
				},
			},
			SSHKey: corev1.LocalObjectReference{
				Name: "test-sshkey",
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
	cd.Spec.ControlPlane = hivev1.MachinePool{
		Name: "master",
		Platform: hivev1.MachinePoolPlatform{
			GCP: &hivev1gcp.MachinePool{
				InstanceType: "n1-standard",
			},
		},
	}
	cd.Spec.Compute = []hivev1.MachinePool{
		{
			Name: defaultWorkerPoolName,
			Platform: hivev1.MachinePoolPlatform{
				GCP: &hivev1gcp.MachinePool{
					InstanceType: "n1-standard",
				},
			},
		},
	}
	cd.Spec.Platform = hivev1.Platform{
		GCP: &hivev1gcp.Platform{
			ProjectID: "my-test-project",
			Region:    "us-central1",
		},
	}

	cd.Spec.PlatformSecrets = hivev1.PlatformSecrets{
		GCP: &hivev1gcp.PlatformSecrets{},
	}

	return cd
}

func validAWSClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform = hivev1.Platform{
		AWS: &hivev1aws.Platform{
			Region: "test-region",
		},
	}
	cd.Spec.PlatformSecrets = hivev1.PlatformSecrets{
		AWS: &hivev1aws.PlatformSecrets{},
	}
	return cd
}

func validAzureClusterDeployment() *hivev1.ClusterDeployment {
	cd := clusterDeploymentTemplate()
	cd.Spec.Platform = hivev1.Platform{
		Azure: &hivev1azure.Platform{
			Region:                      "test-region",
			BaseDomainResourceGroupName: "os4-common",
		},
	}
	cd.Spec.PlatformSecrets = hivev1.PlatformSecrets{
		Azure: &hivev1azure.PlatformSecrets{
			Credentials: corev1.LocalObjectReference{Name: "fake-creds-secret"},
		},
	}
	return cd
}

// Meant to be used to compare new and old as the same values.
func validClusterDeploymentSameValues() *hivev1.ClusterDeployment {
	return validAWSClusterDeployment()
}

func validClusterDeploymentDifferentImmutableValue() *hivev1.ClusterDeployment {
	return &hivev1.ClusterDeployment{
		Spec: hivev1.ClusterDeploymentSpec{
			ClusterName: "DifferentClusterName",
			Compute: []hivev1.MachinePool{
				{
					Name: "SameMachinePoolName",
				},
			},
		},
	}
}

func validClusterDeploymentDifferentMutableValue() *hivev1.ClusterDeployment {
	cd := validAWSClusterDeployment()
	cd.Spec.Compute = []hivev1.MachinePool{
		{
			Name:     defaultWorkerPoolName,
			Replicas: pointer.Int64Ptr(100),
		},
	}
	return cd
}

func TestClusterDeploymentValidatingResource(t *testing.T) {
	// Arrange
	data := ClusterDeploymentValidatingAdmissionHook{}
	expectedPlural := schema.GroupVersionResource{
		Group:    "admission.hive.openshift.io",
		Version:  "v1alpha1",
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
	data := ClusterDeploymentValidatingAdmissionHook{}

	// Act
	err := data.Initialize(nil, nil)

	// Assert
	assert.Nil(t, err)
}

func TestClusterDeploymentValidate(t *testing.T) {
	cases := []struct {
		name            string
		newObject       *hivev1.ClusterDeployment
		newObjectRaw    []byte
		oldObject       *hivev1.ClusterDeployment
		oldObjectRaw    []byte
		operation       admissionv1beta1.Operation
		expectedAllowed bool
		gvr             *metav1.GroupVersionResource
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
			name:      "Test setting installed flag",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Installed = true
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
				Version:  "v1alpha1",
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
				Version:  "v1alpha1",
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
			name: "Test new clusterdeployment with missing SSH key",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validClusterDeploymentWithIngress()
				cd.Spec.SSHKey.Name = ""
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
			name:            "Test invalid managed domain",
			newObject:       clusterDeploymentWithManagedDomain("baz.foo.bbb.com"),
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
			name: "Test managed DNS is invalid on Azure",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAzureClusterDeployment()
				cd.Spec.ManageDNS = true
				cd.Spec.BaseDomain = "bar.foo.aaa.com"
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
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
						SecretRef: corev1.LocalObjectReference{
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
			name:      "Test unallowed update of existing machinepool labels",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Compute[0].Labels = map[string]string{
					"newlabel": "newvalue",
				}

				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name:      "Test unallowed update of existing machinepool taints",
			oldObject: validAWSClusterDeployment(),
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Compute[0].Taints = []corev1.Taint{
					{
						Key:   "testTaint",
						Value: "testTaintVal",
					},
				}

				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
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
				cd.Spec.PlatformSecrets.Azure = nil
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
						Name:      "test-serving-cert",
						SecretRef: corev1.LocalObjectReference{Name: "test-serving-cert-secret"},
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
						Name:      "test-serving-cert",
						SecretRef: corev1.LocalObjectReference{Name: "test-serving-cert-secret"},
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
						Name:      "test-serving-cert",
						SecretRef: corev1.LocalObjectReference{Name: "test-serving-cert-secret"},
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
						SecretRef: corev1.LocalObjectReference{Name: "test-serving-cert-secret"},
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
		// TODO: ensure AWS clusterDeployments provide necessary info
		// for machinesets
		// {
		// 	name: "not setting instance type on AWS machine pool",
		// 	newObject: func() *hivev1.ClusterDeployment {
		// 		cd := validAWSClusterDeployment()
		// 		cd.Spec.Compute = []hivev1.MachinePool{
		// 			{
		// 				Name: "testmachinepool",
		// 				Platform: hivev1.MachinePoolPlatform{
		// 					AWS: &hivev1aws.MachinePoolPlatform{
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
			name:            "valid GCP clusterdeployment",
			newObject:       validGCPClusterDeployment(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: true,
		},
		{
			name: "GCP clusterdeployment with empty controlplane",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validGCPClusterDeployment()
				cd.Spec.ControlPlane = hivev1.MachinePool{}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "GCP clusterdeployment with empty controlplane machinepool platform",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validGCPClusterDeployment()
				cd.Spec.ControlPlane = hivev1.MachinePool{
					Name: "master",
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "not setting instance type on GCP machine pool platform",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validGCPClusterDeployment()
				cd.Spec.Compute = []hivev1.MachinePool{
					{
						Name: "badmachinepool",
						Platform: hivev1.MachinePoolPlatform{
							GCP: &hivev1gcp.MachinePool{
								InstanceType: "",
							},
						},
					},
				}
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "update from valid to invalid machine pool",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validGCPClusterDeployment()
				cd.Spec.Compute = []hivev1.MachinePool{
					{
						Name: "badmachinepool",
						Platform: hivev1.MachinePoolPlatform{
							GCP: &hivev1gcp.MachinePool{
								InstanceType: "",
							},
						},
					},
				}
				return cd
			}(),
			oldObject:       validGCPClusterDeployment(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name: "only allow one compute machine pool for GCP",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validGCPClusterDeployment()
				cd.Spec.Compute = append(cd.Spec.Compute, hivev1.MachinePool{
					Name: "extramachinepool",
					Platform: hivev1.MachinePoolPlatform{
						GCP: &hivev1gcp.MachinePool{
							InstanceType: "someinstancetype",
						},
					},
				})
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
		{
			name: "no update to multiple machine pools for GCP",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validGCPClusterDeployment()
				cd.Spec.Compute = append(cd.Spec.Compute, hivev1.MachinePool{
					Name: "extramachinepool",
					Platform: hivev1.MachinePoolPlatform{
						GCP: &hivev1gcp.MachinePool{
							InstanceType: "someinstancetype",
						},
					},
				})
				return cd
			}(),
			operation:       admissionv1beta1.Update,
			expectedAllowed: false,
		},
		{
			name: "ensure unique compute machine pool names",
			newObject: func() *hivev1.ClusterDeployment {
				cd := validAWSClusterDeployment()
				cd.Spec.Compute = append(cd.Spec.Compute, hivev1.MachinePool{
					Name: "extramachinepool",
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1aws.MachinePoolPlatform{},
					},
				})
				cd.Spec.Compute = append(cd.Spec.Compute, hivev1.MachinePool{
					Name: "extramachinepool",
					Platform: hivev1.MachinePoolPlatform{
						AWS: &hivev1aws.MachinePoolPlatform{},
					},
				})
				return cd
			}(),
			operation:       admissionv1beta1.Create,
			expectedAllowed: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			data := ClusterDeploymentValidatingAdmissionHook{
				validManagedDomains: validTestManagedDomains,
			}

			if tc.gvr == nil {
				tc.gvr = &metav1.GroupVersionResource{
					Group:    "hive.openshift.io",
					Version:  "v1alpha1",
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
	tempFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	domains := []string{
		"first.domain.com",
		"second.domain.com",
		"third.domain.com",
	}
	for _, domain := range domains {
		fmt.Fprintf(tempFile, "     %s     \n", domain)
		fmt.Fprintf(tempFile, "     \n")
	}
	err = tempFile.Close()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	os.Setenv(constants.ManagedDomainsFileEnvVar, tempFile.Name())
	webhook := NewClusterDeploymentValidatingAdmissionHook()
	assert.Equal(t, webhook.validManagedDomains, domains, "valid domains must match expected")
}
