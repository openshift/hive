package util

import (
	"context"
	"os"
	"time"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

const (
	leaseDuration = 120 * time.Second
	renewDeadline = 90 * time.Second
	retryPeriod   = 30 * time.Second
)

func RunWithLeaderElection(ctx context.Context, cfg *rest.Config, lockNS, lockName string, run func(ctx context.Context)) {
	// Leader election code based on:
	// https://github.com/kubernetes/kubernetes/blob/f7e3bcdec2e090b7361a61e21c20b3dbbb41b7f0/staging/src/k8s.io/client-go/examples/leader-election/main.go#L92-L154
	// This gives us ReleaseOnCancel which is not presently exposed in controller-runtime.

	// [Later] Migrated to using resourcelock.New() with ConfigMapsLeasesResourceLock.
	// [Later] Migrated to using LeasesResourceLock.

	id := uuid.New().String()
	leLog := log.WithField("id", id)
	leLog.Info("generated leader election ID")

	kubeClient := kubernetes.NewForConfigOrDie(cfg)
	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		lockNS,
		lockName,
		kubeClient.CoreV1(),
		kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id,
		},
	)
	if err != nil {
		log.WithError(err).Fatal("failed to create lock for leader election config")
	}

	// start the leader election code loop
	leaderelection.RunOrDie(ctx, leaderelection.LeaderElectionConfig{
		Lock:            lock,
		ReleaseOnCancel: true,
		LeaseDuration:   leaseDuration,
		RenewDeadline:   renewDeadline,
		RetryPeriod:     retryPeriod,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				run(ctx)
			},
			OnStoppedLeading: func() {
				// we can do cleanup here if necessary
				leLog.Infof("leader lost")
				os.Exit(0)
			},
			OnNewLeader: func(identity string) {
				if identity == id {
					// We just became the leader
					leLog.Info("became leader")
					return
				}
				log.Infof("current leader: %s", identity)
			},
		},
	})

}
