package swarm

import "context"

// ActualService represents a service's runtime state from Swarm.
//
// Global Mode Comparison Strategy:
// For global mode services, DesiredReplicas is populated from ServiceStatus.DesiredTasks,
// which reflects the actual number of nodes where the service should run. This value is
// dynamic and determined by Swarm at runtime based on node availability and placement
// constraints.
//
// When comparing with DesiredService:
//   - For replicated mode: compare DesiredService.Replicas with ActualService.DesiredReplicas
//   - For global mode: compare ActualService.DesiredReplicas with ActualService.RunningReplicas
//     (DesiredService.Replicas will be 0, indicating "use Swarm's dynamic count")
//
// Use swarm.NormalizeImage() when comparing Image fields to strip digest suffixes.
type ActualService struct {
	Name            string   // Service name (stack prefix stripped if applicable)
	Image           string   // Image reference (may include @sha256:... digest)
	Mode            string   // "replicated", "global", "replicated-job", or "global-job"
	DesiredReplicas int      // Target replica count (from Spec or ServiceStatus)
	RunningReplicas int      // Count of tasks in "running" state
	Configs         []string // Sorted list of config names from running tasks
	Secrets         []string // Sorted list of secret names from running tasks
	UpdateState     string   // UpdateStatus.State when present (e.g., updating, rollback_started)
}

// ActualState represents the complete runtime state of the stack.
type ActualState struct {
	Services map[string]ActualService
}

// Client defines the interface for Swarm API interactions.
// This interface enables mocking in tests.
type Client interface {
	// Ping validates connectivity to the Docker daemon.
	Ping(ctx context.Context) error

	// GetActualState retrieves the current state of services, optionally scoped to a stack.
	GetActualState(ctx context.Context, stackName string) (*ActualState, error)

	// Close releases resources associated with the client.
	Close() error
}
