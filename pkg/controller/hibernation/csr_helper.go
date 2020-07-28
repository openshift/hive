package hibernation

import (
	"crypto/x509"

	certsv1beta1 "k8s.io/api/certificates/v1beta1"
	kubeclient "k8s.io/client-go/kubernetes"

	machineapi "github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
)

//go:generate mockgen -source=./csr_helper.go -destination=./mock/csr_helper_generated.go -package=mock
type csrHelper interface {
	IsApproved(csr *certsv1beta1.CertificateSigningRequest) bool
	Parse(obj *certsv1beta1.CertificateSigningRequest) (*x509.CertificateRequest, error)
	Authorize(machines []machineapi.Machine, nodes kubeclient.Interface, req *certsv1beta1.CertificateSigningRequest, csr *x509.CertificateRequest) error
	Approve(client kubeclient.Interface, csr *certsv1beta1.CertificateSigningRequest) error
}
