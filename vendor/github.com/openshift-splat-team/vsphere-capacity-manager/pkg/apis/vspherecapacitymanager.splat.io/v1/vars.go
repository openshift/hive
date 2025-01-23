package v1

const (
	RESOURCE_ALLOCATION_STRATEGY_RANDOM        = AllocationStrategy("random")
	RESOURCE_ALLOCATION_STRATEGY_UNDERUTILIZED = AllocationStrategy("under-utilized")

	PHASE_FULFILLED Phase = "Fulfilled"
	PHASE_PENDING   Phase = "Pending"
	PHASE_FAILED    Phase = "Failed"
)

type (
	Phase              string
	AllocationStrategy string
	State              string
)
