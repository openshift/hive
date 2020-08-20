package manager

import (
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

//go:generate mockgen -package=mock -destination=./mock/manager_generated.go github.com/openshift/hive/pkg/test/manager Manager

// Manager is only used so that we can use the comment above to generate a mock of the manager.Manager interface.
type Manager interface {
	manager.Manager
}
