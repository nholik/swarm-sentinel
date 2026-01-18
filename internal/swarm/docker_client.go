package swarm

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/docker/docker/client"
)

const defaultAPITimeout = 5 * time.Second

// DockerClient implements Client using the official Docker Go SDK.
type DockerClient struct {
	api     *client.Client
	timeout time.Duration
}

// NewDockerClient initializes a Docker client for the given API host.
func NewDockerClient(host string, timeout time.Duration) (*DockerClient, error) {
	if timeout <= 0 {
		timeout = defaultAPITimeout
	}

	httpClient := &http.Client{Timeout: timeout}

	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(httpClient),
	}
	if host != "" {
		opts = append(opts, client.WithHost(host))
	}

	api, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	return &DockerClient{
		api:     api,
		timeout: timeout,
	}, nil
}

// Ping validates connectivity to the Docker daemon.
func (c *DockerClient) Ping(ctx context.Context) error {
	if c == nil || c.api == nil {
		return errors.New("docker client is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, err := c.api.Ping(ctx)
	return err
}

// GetActualState is implemented in SS-008.
func (c *DockerClient) GetActualState(ctx context.Context) (*ActualState, error) {
	_ = ctx
	return nil, errors.New("actual state collection not implemented")
}
