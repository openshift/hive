package hibernation

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"reflect"
	"strings"

	"github.com/openshift/cluster-api/pkg/apis/machine/v1beta1"
	certificatesv1beta1 "k8s.io/api/certificates/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
)

// copied from github.com/openshift/cluster-machine-approver/csr_check.go
// with modifications for the hibernation resume case

const (
	nodeUser       = "system:node"
	nodeGroup      = "system:nodes"
	nodeUserPrefix = nodeUser + ":"

	nodeBootstrapperUsername = "system:serviceaccount:openshift-machine-config-operator:node-bootstrapper"
)

var workerNodeBootstrapperGroups = sets.NewString(
	"system:serviceaccounts:openshift-machine-config-operator",
	"system:serviceaccounts",
	"system:authenticated",
)

var masterNodeBootstrapperGroups = sets.NewString(
	"system:serviceaccounts:openshift-machine-config-operator",
	"system:authenticated",
)

var kubeletClientUsages = []certificatesv1beta1.KeyUsage{
	certificatesv1beta1.UsageKeyEncipherment,
	certificatesv1beta1.UsageDigitalSignature,
	certificatesv1beta1.UsageClientAuth,
}

var kubeletServerUsages = []certificatesv1beta1.KeyUsage{
	certificatesv1beta1.UsageDigitalSignature,
	certificatesv1beta1.UsageKeyEncipherment,
	certificatesv1beta1.UsageServerAuth,
}

// csrUtility implements the csrHelper interface for use in the hibernation controller
type csrUtility struct{}

func (u *csrUtility) IsApproved(csr *certificatesv1beta1.CertificateSigningRequest) bool {
	for _, condition := range csr.Status.Conditions {
		if condition.Type == certificatesv1beta1.CertificateApproved {
			return true
		}
	}
	return false
}

// parseCSR extracts the CSR from the API object and decodes it.
func (u *csrUtility) Parse(obj *certificatesv1beta1.CertificateSigningRequest) (*x509.CertificateRequest, error) {
	// extract PEM from request object
	block, _ := pem.Decode(obj.Spec.Request)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return nil, fmt.Errorf("PEM block type must be CERTIFICATE REQUEST")
	}
	return x509.ParseCertificateRequest(block.Bytes)
}

func (u *csrUtility) Approve(client kubeclient.Interface, csr *certificatesv1beta1.CertificateSigningRequest) error {
	if u.IsApproved(csr) {
		return nil
	}
	// Add approved condition
	csr.Status.Conditions = append(csr.Status.Conditions, certificatesv1beta1.CertificateSigningRequestCondition{
		Type:           certificatesv1beta1.CertificateApproved,
		Reason:         "KubectlApprove",
		Message:        "This CSR was automatically approved by Hive",
		LastUpdateTime: metav1.Now(),
	})
	_, err := client.CertificatesV1beta1().CertificateSigningRequests().UpdateApproval(context.TODO(), csr, metav1.UpdateOptions{})
	return err
}

// Authorize authorizes the CertificateSigningRequest req for a node's client or server certificate.
// csr should be the parsed CSR from req.Spec.Request.
//
// For client certificates, when the flow is not globally disabled:
// The only information contained in the CSR is the future name of the node.  Thus we perform a best effort check:
//
// 1. User is the node bootstrapper
// 2. Node exists
// 3. Use machine API internal DNS to locate matching machine based on node name
// 4. CSR is meant for node client auth based on usage, CN, etc
//
// For server certificates:
// Names contained in the CSR are checked against addresses in the corresponding node's machine status.
func (*csrUtility) Authorize(
	machines []v1beta1.Machine,
	client kubeclient.Interface,
	req *certificatesv1beta1.CertificateSigningRequest,
	csr *x509.CertificateRequest,
) error {
	if req == nil || csr == nil {
		return fmt.Errorf("Invalid request")
	}

	if isNodeClientCert(req, csr) {
		return authorizeNodeClientCSR(machines, client, req, csr)
	}

	// node serving cert validation after this point

	nodeAsking, err := validateCSRContents(req, csr)
	if err != nil {
		return err
	}

	// Fall back to the original machine-api based authorization scheme.
	klog.Infof("Falling back to machine-api authorization for %s", nodeAsking)

	// Check that we have a registered node with the request name
	targetMachine, ok := findMatchingMachineFromNodeRef(nodeAsking, machines)
	if !ok {
		return fmt.Errorf("No target machine for node %q", nodeAsking)
	}

	// SAN checks for both DNS and IPs, e.g.,
	// DNS:ip-10-0-152-205, DNS:ip-10-0-152-205.ec2.internal, IP Address:10.0.152.205, IP Address:10.0.152.205
	// All names in the request must correspond to addresses assigned to a single machine.
	for _, san := range csr.DNSNames {
		if len(san) == 0 {
			continue
		}
		var attemptedAddresses []string
		var foundSan bool
		for _, addr := range targetMachine.Status.Addresses {
			switch addr.Type {
			case corev1.NodeInternalDNS, corev1.NodeExternalDNS, corev1.NodeHostName:
				if san == addr.Address {
					foundSan = true
					break
				} else {
					attemptedAddresses = append(attemptedAddresses, addr.Address)
				}
			default:
			}
		}
		// The CSR requested a DNS name that did not belong to the machine
		if !foundSan {
			return fmt.Errorf("DNS name '%s' not in machine names: %s", san, strings.Join(attemptedAddresses, " "))
		}
	}

	for _, san := range csr.IPAddresses {
		if len(san) == 0 {
			continue
		}
		var attemptedAddresses []string
		var foundSan bool
		for _, addr := range targetMachine.Status.Addresses {
			switch addr.Type {
			case corev1.NodeInternalIP, corev1.NodeExternalIP:
				if san.String() == addr.Address {
					foundSan = true
					break
				} else {
					attemptedAddresses = append(attemptedAddresses, addr.Address)
				}
			default:
			}
		}
		// The CSR requested an IP name that did not belong to the machine
		if !foundSan {
			return fmt.Errorf("IP address '%s' not in machine addresses: %s", san, strings.Join(attemptedAddresses, " "))
		}
	}

	return nil
}

func validateCSRContents(req *certificatesv1beta1.CertificateSigningRequest, csr *x509.CertificateRequest) (string, error) {
	if !strings.HasPrefix(req.Spec.Username, nodeUserPrefix) {
		return "", fmt.Errorf("%q doesn't match expected prefix: %q", req.Spec.Username, nodeUserPrefix)
	}

	nodeAsking := strings.TrimPrefix(req.Spec.Username, nodeUserPrefix)
	if len(nodeAsking) == 0 {
		return "", fmt.Errorf("Empty name")
	}

	// Check groups, we need at least:
	// - system:nodes
	// - system:authenticated
	if len(req.Spec.Groups) < 2 {
		return "", fmt.Errorf("Too few groups")
	}
	groupSet := sets.NewString(req.Spec.Groups...)
	if !groupSet.HasAll(nodeGroup, "system:authenticated") {
		return "", fmt.Errorf("%q not in %q and %q", groupSet, "system:authenticated", nodeGroup)
	}

	// Check usages, we need only:
	// - digital signature
	// - key encipherment
	// - server auth
	if !hasExactUsages(req, kubeletServerUsages) {
		return "", fmt.Errorf("Unexpected usages: %v", req.Spec.Usages)
	}

	// Check subject: O = system:nodes, CN = system:node:ip-10-0-152-205.ec2.internal
	if csr.Subject.CommonName != req.Spec.Username {
		return "", fmt.Errorf("Mismatched CommonName %s != %s", csr.Subject.CommonName, req.Spec.Username)
	}

	var hasOrg bool
	for i := range csr.Subject.Organization {
		if csr.Subject.Organization[i] == nodeGroup {
			hasOrg = true
			break
		}
	}
	if !hasOrg {
		return "", fmt.Errorf("Organization %v doesn't include %s", csr.Subject.Organization, nodeGroup)
	}

	return nodeAsking, nil
}

func authorizeNodeClientCSR(machines []v1beta1.Machine, client kubeclient.Interface, req *certificatesv1beta1.CertificateSigningRequest, csr *x509.CertificateRequest) error {

	if !isReqFromNodeBootstrapper(req) {
		return fmt.Errorf("CSR %s for node client cert has wrong user %s or groups %s", req.Name, req.Spec.Username, sets.NewString(req.Spec.Groups...))
	}

	nodeName := strings.TrimPrefix(csr.Subject.CommonName, nodeUserPrefix)
	if len(nodeName) == 0 {
		return fmt.Errorf("CSR %s has empty node name", req.Name)
	}

	_, err := client.CoreV1().Nodes().Get(context.Background(), nodeName, metav1.GetOptions{})
	switch {
	case err == nil:
		// change for hive hibernation: it is expected that the node already exists
	case errors.IsNotFound(err):
		// change for hive hibernation: we only approve csrs for nodes that exist
		return fmt.Errorf("node %s does not exist in the cluster, cannot be approved", nodeName)
	default:
		return fmt.Errorf("failed to check if node %s already exists: %v", nodeName, err)
	}

	_, ok := findMatchingMachineFromInternalDNS(nodeName, machines)
	if !ok {
		return fmt.Errorf("failed to find machine for node %s", nodeName)
	}

	return nil // approve node client cert
}

func isReqFromNodeBootstrapper(req *certificatesv1beta1.CertificateSigningRequest) bool {
	groups := sets.NewString(req.Spec.Groups...)
	return req.Spec.Username == nodeBootstrapperUsername &&
		(workerNodeBootstrapperGroups.Equal(groups) ||
			masterNodeBootstrapperGroups.Equal(groups))
}

func findMatchingMachineFromNodeRef(nodeName string, machines []v1beta1.Machine) (v1beta1.Machine, bool) {
	for _, machine := range machines {
		if machine.Status.NodeRef != nil && machine.Status.NodeRef.Name == nodeName {
			return machine, true
		}
	}
	return v1beta1.Machine{}, false
}

func findMatchingMachineFromInternalDNS(nodeName string, machines []v1beta1.Machine) (v1beta1.Machine, bool) {
	for _, machine := range machines {
		for _, address := range machine.Status.Addresses {
			if address.Type == corev1.NodeInternalDNS && address.Address == nodeName {
				return machine, true
			}
		}
	}
	return v1beta1.Machine{}, false
}

func keyUsageSliceToStringSlice(usages []certificatesv1beta1.KeyUsage) []string {
	result := make([]string, len(usages))
	for i := range usages {
		result[i] = string(usages[i])
	}
	return result
}

func hasExactUsages(csr *certificatesv1beta1.CertificateSigningRequest, usages []certificatesv1beta1.KeyUsage) bool {
	return sets.NewString(keyUsageSliceToStringSlice(csr.Spec.Usages)...).Equal(sets.NewString(keyUsageSliceToStringSlice(usages)...))
}

func isNodeClientCert(csr *certificatesv1beta1.CertificateSigningRequest, x509cr *x509.CertificateRequest) bool {
	if !reflect.DeepEqual([]string{"system:nodes"}, x509cr.Subject.Organization) {
		return false
	}
	if (len(x509cr.DNSNames) > 0) || (len(x509cr.EmailAddresses) > 0) || (len(x509cr.IPAddresses) > 0) {
		return false
	}
	if !hasExactUsages(csr, kubeletClientUsages) {
		return false
	}
	if !strings.HasPrefix(x509cr.Subject.CommonName, "system:node:") {
		return false
	}
	return true
}
