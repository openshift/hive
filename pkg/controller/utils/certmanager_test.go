package utils

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/pkg/util/scheme"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetDomainsForCertBundle(t *testing.T) {
	tests := []struct {
		name            string
		cd              *hivev1.ClusterDeployment
		bundle          hivev1.CertificateBundleSpec
		expectedDomains []string
	}{
		{
			name: "default control plane cert",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Default: "primary",
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name: "primary",
			},
			expectedDomains: []string{"api.test-cluster.example.com"},
		},
		{
			name: "additional control plane cert",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Additional: []hivev1.ControlPlaneAdditionalCertificate{
								{Name: "extra", Domain: "custom.example.com"},
							},
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name: "extra",
			},
			expectedDomains: []string{"custom.example.com"},
		},
		{
			name: "ingress cert adds wildcard",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					Ingress: []hivev1.ClusterIngress{
						{
							Name:               "default",
							Domain:             "apps.test-cluster.example.com",
							ServingCertificate: "ingress-cert",
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name: "ingress-cert",
			},
			expectedDomains: []string{"*.apps.test-cluster.example.com"},
		},
		{
			name: "ingress cert already wildcard",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					Ingress: []hivev1.ClusterIngress{
						{
							Name:               "default",
							Domain:             "*.apps.test-cluster.example.com",
							ServingCertificate: "ingress-cert",
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name: "ingress-cert",
			},
			expectedDomains: []string{"*.apps.test-cluster.example.com"},
		},
		{
			name: "combined default + ingress",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Default: "primary",
						},
					},
					Ingress: []hivev1.ClusterIngress{
						{
							Name:               "default",
							Domain:             "apps.test-cluster.example.com",
							ServingCertificate: "primary",
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name: "primary",
			},
			expectedDomains: []string{
				"api.test-cluster.example.com",
				"*.apps.test-cluster.example.com",
			},
		},
		{
			name: "non-matching bundle returns empty",
			cd: &hivev1.ClusterDeployment{
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Default: "other-cert",
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name: "primary",
			},
			expectedDomains: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			domains := GetDomainsForCertBundle(tt.bundle, tt.cd)
			assert.Equal(t, tt.expectedDomains, domains)
		})
	}
}

func TestEnsureCertManagerCertificate(t *testing.T) {
	s := scheme.GetScheme()
	logger := log.WithField("test", true)

	tests := []struct {
		name           string
		cd             *hivev1.ClusterDeployment
		bundle         hivev1.CertificateBundleSpec
		existingCert   *certmanagerv1.Certificate
		expectCreated  bool
		expectError    bool
	}{
		{
			name: "creates certificate when Generate=true and no existing cert",
			cd: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					UID:       "test-uid",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Default: "primary",
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name:                 "primary",
				Generate:             true,
				CertificateSecretRef: corev1.LocalObjectReference{Name: "primary-cert-secret"},
			},
			expectCreated: true,
		},
		{
			name: "skips when Generate=false",
			cd: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Default: "primary",
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name:                 "primary",
				Generate:             false,
				CertificateSecretRef: corev1.LocalObjectReference{Name: "primary-cert-secret"},
			},
			expectCreated: false,
		},
		{
			name: "skips when certificate already exists",
			cd: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
					UID:       "test-uid",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
					ControlPlaneConfig: hivev1.ControlPlaneConfigSpec{
						ServingCertificates: hivev1.ControlPlaneServingCertificateSpec{
							Default: "primary",
						},
					},
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name:                 "primary",
				Generate:             true,
				CertificateSecretRef: corev1.LocalObjectReference{Name: "primary-cert-secret"},
			},
			existingCert: &certmanagerv1.Certificate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-primary",
					Namespace: "test-ns",
				},
			},
			expectCreated: false,
		},
		{
			name: "skips when no domains found",
			cd: &hivev1.ClusterDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-ns",
				},
				Spec: hivev1.ClusterDeploymentSpec{
					ClusterName: "test-cluster",
					BaseDomain:  "example.com",
				},
			},
			bundle: hivev1.CertificateBundleSpec{
				Name:                 "unmatched-bundle",
				Generate:             true,
				CertificateSecretRef: corev1.LocalObjectReference{Name: "some-secret"},
			},
			expectCreated: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []client.Object{tt.cd}
			if tt.existingCert != nil {
				objs = append(objs, tt.existingCert)
			}
			c := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()

			err := EnsureCertManagerCertificate(context.TODO(), c, s, tt.cd, tt.bundle, logger)

			if tt.expectError {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			certName := "test-cluster-" + tt.bundle.Name
			cert := &certmanagerv1.Certificate{}
			getErr := c.Get(context.TODO(), types.NamespacedName{Name: certName, Namespace: tt.cd.Namespace}, cert)

			if tt.expectCreated {
				require.NoError(t, getErr)
				assert.Equal(t, tt.bundle.CertificateSecretRef.Name, cert.Spec.SecretName)
				assert.Equal(t, CertManagerIssuerName, cert.Spec.IssuerRef.Name)
				assert.Equal(t, CertManagerIssuerKind, cert.Spec.IssuerRef.Kind)
				assert.NotEmpty(t, cert.Spec.DNSNames)
				assert.Equal(t, 2048, cert.Spec.PrivateKey.Size)
			} else if tt.existingCert == nil && !tt.bundle.Generate {
				assert.Error(t, getErr, "certificate should not have been created")
			}
		})
	}
}
