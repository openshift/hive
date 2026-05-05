# Module atlas

## Responsibility

Shared leader-election utility for Hive binary entry points (`cmd/manager`, `cmd/operator`). Wraps Kubernetes client-go leader election with Lease-based resource locks.

## Public Interface/API

- `RunWithLeaderElection(ctx context.Context, cfg *rest.Config, lockNS, lockName string, run func(ctx context.Context))` — acquires a Lease lock in the given namespace, calls `run` when leadership is acquired, and exits the process when leadership is lost.

## Internal Dependencies

- `k8s.io/client-go/kubernetes` — Kubernetes clientset for lock operations
- `k8s.io/client-go/rest` — REST config type
- `k8s.io/client-go/tools/leaderelection` — leader election framework
- `k8s.io/client-go/tools/leaderelection/resourcelock` — Lease-based resource lock
- `github.com/google/uuid` — unique leader identity generation (external)
- `github.com/sirupsen/logrus` — structured logging (external)

## Capabilities

- Provides a single reusable leader election entry point for all Hive binaries
- Uses `LeasesResourceLock` for leader election (migrated from ConfigMaps)
- Generates a unique UUID identity per process instance
- Supports `ReleaseOnCancel` for graceful leadership handoff
- Configures lease duration (120s), renew deadline (90s), and retry period (30s)

## Understanding Score

0.9
