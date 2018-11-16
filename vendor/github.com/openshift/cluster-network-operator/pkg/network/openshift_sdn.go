package network

import (
	"os"
	"path/filepath"

	"github.com/ghodss/yaml"
	"github.com/pkg/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	uns "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	legacyconfigv1 "github.com/openshift/api/legacyconfig/v1"
	cpv1 "github.com/openshift/api/openshiftcontrolplane/v1"
	netv1 "github.com/openshift/cluster-network-operator/pkg/apis/networkoperator/v1"
	"github.com/openshift/cluster-network-operator/pkg/render"
)

// NodeNameMagicString is substituted at runtime for the
// real nodename
const NodeNameMagicString = "%%NODENAME%%"

// renderOpenshiftSDN returns the manifests for the openshift-sdn.
// This creates
// - the ClusterNetwork object
// - the sdn namespace
// - the sdn daemonset
// - the openvswitch daemonset
// and some other small things.
func renderOpenshiftSDN(conf *netv1.NetworkConfig, manifestDir string) ([]*uns.Unstructured, error) {
	operConfig := conf.Spec
	c := operConfig.DefaultNetwork.OpenshiftSDNConfig

	objs := []*uns.Unstructured{}

	// render the manifests on disk
	data := render.MakeRenderData()
	data.Data["InstallOVS"] = (c.UseExternalOpenvswitch == nil || *c.UseExternalOpenvswitch == false)
	data.Data["NodeImage"] = os.Getenv("NODE_IMAGE")
	data.Data["HypershiftImage"] = os.Getenv("HYPERSHIFT_IMAGE")

	operCfg, err := controllerConfig(conf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build controller config")
	}
	data.Data["NetworkControllerConfig"] = operCfg

	nodeCfg, err := nodeConfig(conf)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build node config")
	}
	data.Data["NodeConfig"] = nodeCfg

	manifests, err := render.RenderDir(filepath.Join(manifestDir, "network/openshift-sdn"), &data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to render manifests")
	}

	objs = append(objs, manifests...)
	return objs, nil
}

// validateOpenshiftSDN checks that the openshift-sdn specific configuration
// is basically sane.
func validateOpenshiftSDN(conf *netv1.NetworkConfig) []error {
	out := []error{}
	c := conf.Spec
	sc := c.DefaultNetwork.OpenshiftSDNConfig
	if sc == nil {
		out = append(out, errors.Errorf("OpenshiftSDNConfig cannot be nil"))
		return out
	}

	if len(c.ClusterNetworks) == 0 {
		out = append(out, errors.Errorf("ClusterNetworks cannot be empty"))
	}

	if sdnPluginName(sc.Mode) == "" {
		out = append(out, errors.Errorf("invalid openshift-sdn mode %q", sc.Mode))
	}

	if sc.VXLANPort != nil && (*sc.VXLANPort < 1 || *sc.VXLANPort > 65535) {
		out = append(out, errors.Errorf("invalid VXLANPort %d", *sc.VXLANPort))
	}

	if sc.MTU != nil && (*sc.MTU < 576 || *sc.MTU > 65536) {
		out = append(out, errors.Errorf("invalid MTU %d", *sc.MTU))
	}

	return out
}

func sdnPluginName(n netv1.SDNMode) string {
	switch n {
	case netv1.SDNModeSubnet:
		return "redhat/openshift-ovs-subnet"
	case netv1.SDNModeMultitenant:
		return "redhat/openshift-ovs-multitenant"
	case netv1.SDNModePolicy:
		return "redhat/openshift-ovs-networkpolicy"
	}
	return ""
}

// controllerConfig builds the contents of controller-config.yaml
// for the controller
func controllerConfig(conf *netv1.NetworkConfig) (string, error) {
	c := conf.Spec.DefaultNetwork.OpenshiftSDNConfig

	// generate master network configuration
	ippools := []cpv1.ClusterNetworkEntry{}
	for _, net := range conf.Spec.ClusterNetworks {
		ippools = append(ippools, cpv1.ClusterNetworkEntry{CIDR: net.CIDR, HostSubnetLength: net.HostSubnetLength})
	}

	var vxlanPort uint32 = 4789
	if c.VXLANPort != nil {
		vxlanPort = *c.VXLANPort
	}

	cfg := cpv1.OpenShiftControllerManagerConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "openshiftcontrolplane.config.openshift.io/v1",
			Kind:       "OpenShiftControllerManagerConfig",
		},
		// no ObjectMeta - not an API object

		Network: cpv1.NetworkControllerConfig{
			NetworkPluginName:  sdnPluginName(c.Mode),
			ClusterNetworks:    ippools,
			ServiceNetworkCIDR: conf.Spec.ServiceNetwork,
			VXLANPort:          vxlanPort,
		},
	}

	buf, err := yaml.Marshal(cfg)
	return string(buf), err
}

// nodeConfig builds the (yaml text of) the NodeConfig object
// consumed by the sdn node process
func nodeConfig(conf *netv1.NetworkConfig) (string, error) {
	operConfig := conf.Spec
	c := conf.Spec.DefaultNetwork.OpenshiftSDNConfig
	kpc := operConfig.KubeProxyConfig

	mtu := uint32(1450)
	if c.MTU != nil {
		mtu = *c.MTU
	}

	bindAddress := "0.0.0.0"
	if kpc != nil && kpc.BindAddress != "" {
		bindAddress = kpc.BindAddress
	}

	result := legacyconfigv1.NodeConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "NodeConfig",
		},
		NodeName: NodeNameMagicString,
		NetworkConfig: legacyconfigv1.NodeNetworkConfig{
			NetworkPluginName: sdnPluginName(c.Mode),
			MTU:               mtu,
		},
		// ServingInfo is used by both the proxy and metrics components
		ServingInfo: legacyconfigv1.ServingInfo{
			// These files are bind-mounted in at a hard-coded location
			CertInfo: legacyconfigv1.CertInfo{
				CertFile: "/etc/origin/node/server.crt",
				KeyFile:  "/etc/origin/node/server.key",
			},
			ClientCA:    "/etc/origin/node/ca.crt",
			BindAddress: bindAddress + ":10251", // port is unused
		},
	}

	if kpc != nil && kpc.IptablesSyncPeriod != "" {
		result.IPTablesSyncPeriod = kpc.IptablesSyncPeriod
	}
	if kpc != nil && len(kpc.ProxyArguments) > 0 {
		result.ProxyArguments = kpc.ProxyArguments
	}

	buf, err := yaml.Marshal(result)
	return string(buf), err

}
