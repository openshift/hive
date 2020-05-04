// Package vsphere contains vSphere-specific structures for installer
// configuration and management.
// +k8s:deepcopy-gen=package,register
// +k8s:conversion-gen=github.com/openshift/hive/pkg/apis/hive
package vsphere

// Name is name for the vsphere platform.
const Name string = "vsphere"
