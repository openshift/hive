package utils

import (
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// Watcher is a smaller interface of controller.Controller
// that only provides function to add watches.
// This allows reconcilers to start new watches when required.
type Watcher interface {
	Watch(src source.Source) error
}

// watcherInjectable interface allows controller to be injected
// to a reconciler at runtime.
type watcherInjectable interface {
	SetWatcher(Watcher)
}

// InjectWatcher injects the reconciler that implements the watcherInjectable
// interface with the provided controller.
// If the reconciler passed does not implement the interface, it returns nil
func InjectWatcher(r reconcile.Reconciler, c controller.Controller) {
	inject, ok := r.(watcherInjectable)
	if !ok {
		return
	}
	inject.SetWatcher(c)
}
