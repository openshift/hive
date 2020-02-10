package v1migration

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ownerRefsFilename = "owner-refs.yaml"
)

type ownerRef struct {
	Group     string                `json:"group"`
	Version   string                `json:"version"`
	Resource  string                `json:"resource"`
	Namespace string                `json:"namespace"`
	Name      string                `json:"name"`
	Ref       metav1.OwnerReference `json:"ref"`
}
