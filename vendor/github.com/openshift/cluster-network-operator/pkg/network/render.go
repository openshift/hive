package network

import (
	"log"

	"github.com/pkg/errors"

	netv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"

	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func Render(conf *netv1.NetworkConfig, manifestDir string) ([]*uns.Unstructured, error) {
	log.Printf("Starting render phase")
	objs := []*uns.Unstructured{}

	// render default network
	o, err := RenderDefaultNetwork(conf, manifestDir)
	if err != nil {
		return nil, err
	}
	objs = append(objs, o...)

	// render kube-proxy
	// TODO: kube-proxy

	// render additional networks
	// TODO: extra networks

	log.Printf("Render phase done, rendered %d objects", len(objs))
	return objs, nil
}

// Validate checks that the supplied configuration is reasonable.
func Validate(conf *netv1.NetworkConfig) error {
	errs := []error{}

	errs = append(errs, ValidateDefaultNetwork(conf)...)

	if len(errs) > 0 {
		return errors.Errorf("invalid configuration: %v", errs)
	}
	return nil
}

// ValidateDefaultNetwork validates whichever network is specified
// as the default network.
func ValidateDefaultNetwork(conf *netv1.NetworkConfig) []error {
	switch conf.Spec.DefaultNetwork.Type {
	case netv1.NetworkTypeOpenshiftSDN:
		return validateOpenshiftSDN(conf)
	default:
		return []error{errors.Errorf("unknown or unsupported NetworkType: %s", conf.Spec.DefaultNetwork.Type)}
	}
}

// RenderDefaultNetwork generates the manifests corresponding to the requested
// default network
func RenderDefaultNetwork(conf *netv1.NetworkConfig, manifestDir string) ([]*uns.Unstructured, error) {
	dn := conf.Spec.DefaultNetwork
	if errs := ValidateDefaultNetwork(conf); len(errs) > 0 {
		return nil, errors.Errorf("invalid Default Network configuration: %v", errs)
	}

	switch dn.Type {
	case netv1.NetworkTypeOpenshiftSDN:
		return renderOpenshiftSDN(conf, manifestDir)
	}

	return nil, errors.Errorf("unknown or unsupported NetworkType: %s", dn.Type)
}
