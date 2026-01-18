package swarm

import (
	"context"

	dockertypes "github.com/docker/docker/api/types"
	swarmtypes "github.com/docker/docker/api/types/swarm"
)

// dockerAPI defines the subset of Docker client operations used by DockerClient.
// This interface enables unit testing without a real Docker daemon by allowing
// mock implementations to be injected.
//
// To use in tests:
//
//	type mockDockerAPI struct {
//	    services []swarmtypes.Service
//	    tasks    []swarmtypes.Task
//	    pingErr  error
//	}
//
//	func (m *mockDockerAPI) Ping(ctx context.Context) (dockertypes.Ping, error) {
//	    return dockertypes.Ping{}, m.pingErr
//	}
//
//	// ... implement other methods
//
//	client := &DockerClient{api: &mockDockerAPI{...}, timeout: 30*time.Second}
type dockerAPI interface {
	// Ping checks connectivity to the Docker daemon.
	Ping(ctx context.Context) (dockertypes.Ping, error)

	// ServiceList returns services matching the given options.
	ServiceList(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error)

	// TaskList returns tasks matching the given options.
	TaskList(ctx context.Context, options dockertypes.TaskListOptions) ([]swarmtypes.Task, error)

	// Close releases resources associated with the client.
	Close() error
}

// Ensure the official Docker client satisfies our interface at compile time.
// This is a compile-time check only and doesn't affect runtime behavior.
var _ dockerAPI = (*dockerClientAdapter)(nil)

// dockerClientAdapter wraps the official Docker client to satisfy the dockerAPI interface.
// The official client.Client has methods with different signatures for Close(), so we
// need this adapter.
type dockerClientAdapter struct {
	client dockerClientInterface
}

// dockerClientInterface captures the methods we use from *client.Client.
type dockerClientInterface interface {
	Ping(ctx context.Context) (dockertypes.Ping, error)
	ServiceList(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error)
	TaskList(ctx context.Context, options dockertypes.TaskListOptions) ([]swarmtypes.Task, error)
	Close() error
}

func (a *dockerClientAdapter) Ping(ctx context.Context) (dockertypes.Ping, error) {
	return a.client.Ping(ctx)
}

func (a *dockerClientAdapter) ServiceList(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error) {
	return a.client.ServiceList(ctx, options)
}

func (a *dockerClientAdapter) TaskList(ctx context.Context, options dockertypes.TaskListOptions) ([]swarmtypes.Task, error) {
	return a.client.TaskList(ctx, options)
}

func (a *dockerClientAdapter) Close() error {
	return a.client.Close()
}
