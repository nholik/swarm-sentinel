package swarm

import "context"

// ActualService represents a service's runtime state from Swarm.
type ActualService struct {
	Name            string
	Image           string
	Mode            string
	DesiredReplicas int
	RunningReplicas int
	Configs         []string
	Secrets         []string
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

	// GetActualState retrieves the current state of all services.
	GetActualState(ctx context.Context) (*ActualState, error)
}
