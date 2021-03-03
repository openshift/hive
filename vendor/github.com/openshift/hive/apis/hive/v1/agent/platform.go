package agent

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// BareMetalPlatform defines agent based install configuration specific to bare metal clusters.
// Can only be used with spec.installStrategy.agent.
type BareMetalPlatform struct {

	// APIVIP is the virtual IP used to reach the OpenShift cluster's API.
	APIVIP string `json:"apiVIP"`

	// APIVIPDNSName is the domain name used to reach the OpenShift cluster API.
	// +optional
	APIVIPDNSName string `json:"apiVIPDNSName,omitempty"`

	// IngressVIP is the virtual IP used for cluster ingress traffic.
	IngressVIP string `json:"ingressVIP"`

	// AgentSelector is a label selector used for associating relevant custom resources with this cluster.
	// (Agent, BareMetalHost, etc)
	AgentSelector metav1.LabelSelector `json:"agentSelector"`
}
