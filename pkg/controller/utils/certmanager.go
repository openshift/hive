package utils

import (
	"context"
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"

	certmanagerv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	CertManagerIssuerName = "public-issuer"
	CertManagerIssuerKind = "ClusterIssuer"
	certDuration          = 90 * 24 * time.Hour
	certRenewBefore       = 30 * 24 * time.Hour
)

// EnsureCertManagerCertificate creates a cert-manager Certificate CR for the given
// CertificateBundle if one does not already exist. Returns true if a Certificate was
// created or already exists, false if domains could not be determined.
func EnsureCertManagerCertificate(
	ctx context.Context,
	c client.Client,
	scheme *runtime.Scheme,
	cd *hivev1.ClusterDeployment,
	bundle hivev1.CertificateBundleSpec,
	cdLog log.FieldLogger,
) error {
	if !bundle.Generate {
		return nil
	}

	domains := GetDomainsForCertBundle(bundle, cd)
	if len(domains) == 0 {
		cdLog.WithField("bundle", bundle.Name).Debug("no domains found for certificate bundle, skipping cert-manager Certificate creation")
		return nil
	}

	certName := fmt.Sprintf("%s-%s", cd.Name, bundle.Name)
	certName = strings.ToLower(certName)

	existing := &certmanagerv1.Certificate{}
	err := c.Get(ctx, types.NamespacedName{Name: certName, Namespace: cd.Namespace}, existing)
	if err == nil {
		cdLog.WithField("certificate", certName).Debug("cert-manager Certificate already exists")
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to check for existing cert-manager Certificate %s: %w", certName, err)
	}

	cert := buildCertManagerCertificate(certName, cd.Namespace, bundle.CertificateSecretRef.Name, domains)

	if err := controllerutil.SetControllerReference(cd, cert, scheme); err != nil {
		return fmt.Errorf("failed to set owner reference on cert-manager Certificate %s: %w", certName, err)
	}

	cdLog.WithFields(log.Fields{
		"certificate": certName,
		"domains":     domains,
		"secretName":  bundle.CertificateSecretRef.Name,
	}).Info("creating cert-manager Certificate for certificate bundle")

	if err := c.Create(ctx, cert); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("failed to create cert-manager Certificate %s: %w", certName, err)
	}

	return nil
}

// GetDomainsForCertBundle extracts the DNS names that should be included in a certificate
// for the given CertificateBundle, based on the ClusterDeployment's control plane and
// ingress configuration.
func GetDomainsForCertBundle(bundle hivev1.CertificateBundleSpec, cd *hivev1.ClusterDeployment) []string {
	var domains []string

	if cd.Spec.ControlPlaneConfig.ServingCertificates.Default == bundle.Name {
		controlPlaneDomain := fmt.Sprintf("api.%s.%s", cd.Spec.ClusterName, cd.Spec.BaseDomain)
		domains = append(domains, controlPlaneDomain)
	}

	for _, additional := range cd.Spec.ControlPlaneConfig.ServingCertificates.Additional {
		if additional.Name == bundle.Name {
			domains = append(domains, additional.Domain)
		}
	}

	for _, ingress := range cd.Spec.Ingress {
		if ingress.ServingCertificate == bundle.Name {
			ingressDomain := ingress.Domain
			if !strings.HasPrefix(ingressDomain, "*.") {
				ingressDomain = fmt.Sprintf("*.%s", ingress.Domain)
			}
			domains = append(domains, ingressDomain)
		}
	}

	return domains
}

func buildCertManagerCertificate(name, namespace, secretName string, domains []string) *certmanagerv1.Certificate {
	return &certmanagerv1.Certificate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Certificate",
			APIVersion: "cert-manager.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: certmanagerv1.CertificateSpec{
			Subject: &certmanagerv1.X509Subject{
				Organizations: []string{"Red Hat - Open Cluster Manager"},
			},
			Usages: []certmanagerv1.KeyUsage{
				certmanagerv1.UsageServerAuth,
				certmanagerv1.UsageClientAuth,
			},
			Duration:    &metav1.Duration{Duration: certDuration},
			RenewBefore: &metav1.Duration{Duration: certRenewBefore},
			PrivateKey: &certmanagerv1.CertificatePrivateKey{
				Algorithm:      certmanagerv1.RSAKeyAlgorithm,
				Encoding:       certmanagerv1.PKCS1,
				Size:           2048,
				RotationPolicy: certmanagerv1.RotationPolicyAlways,
			},
			DNSNames:   domains,
			SecretName: secretName,
			IssuerRef: cmmeta.IssuerReference{
				Name:  CertManagerIssuerName,
				Group: "cert-manager.io",
				Kind:  CertManagerIssuerKind,
			},
		},
	}
}
