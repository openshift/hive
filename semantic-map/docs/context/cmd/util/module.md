# Module atlas

## Responsibility

Shared utility package for Hive command binaries, providing a reusable leader election wrapper used by both `cmd/manager` and `cmd/operator`.

## Public Interface/API

- `func RunWithLeaderElection(ctx context.Context, cfg *rest.Config, lockNS, lockName string, run func(ctx context.Context))` — runs the provided function only when this instance holds the leader lease; exits the process when leadership is lost

## Internal Dependencies

- `github.com/google/uuid` — unique leader election identity
- `github.com/sirupsen/logrus` — structured logging
- `k8s.io/client-go/kubernetes` — kube client for lease management
- `k8s.io/client-go/rest` — REST config type
- `k8s.io/client-go/tools/leaderelection` — leader election loop
- `k8s.io/client-go/tools/leaderelection/resourcelock` — Lease-based resource lock

## Capabilities

- Wraps Kubernetes leader election using Lease resource locks
- Generates a UUID-based identity per instance
- Configures lease duration (120s), renew deadline (90s), and retry period (30s)
- Supports `ReleaseOnCancel` for graceful leadership handoff
- Exits process on leadership loss

## Understanding Score

0.9
