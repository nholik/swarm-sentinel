package swarm

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	swarmtypes "github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/tlsconfig"
)

const defaultAPITimeout = 5 * time.Second

// TLSConfig describes the client TLS configuration.
type TLSConfig struct {
	Enabled  bool
	Verify   bool
	CAFile   string
	CertFile string
	KeyFile  string
}

// DockerClient implements Client using the official Docker Go SDK.
type DockerClient struct {
	api     *client.Client
	timeout time.Duration
}

// NewDockerClient initializes a Docker client for the given API host.
func NewDockerClient(host string, timeout time.Duration, tls TLSConfig) (*DockerClient, error) {
	if timeout <= 0 {
		timeout = defaultAPITimeout
	}

	transport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, errors.New("default HTTP transport is not *http.Transport")
	}

	httpClient := &http.Client{
		Timeout:   timeout,
		Transport: transport.Clone(),
	}

	if tls.Enabled {
		if tls.CertFile == "" || tls.KeyFile == "" {
			return nil, errors.New("docker tls enabled but cert/key are required")
		}
		if tls.Verify && tls.CAFile == "" {
			return nil, errors.New("docker tls verify enabled but CA is required")
		}

		transport, ok := httpClient.Transport.(*http.Transport)
		if !ok {
			return nil, errors.New("docker client transport is not *http.Transport")
		}

		tlsConfig, err := tlsconfig.Client(tlsconfig.Options{
			CAFile:             tls.CAFile,
			CertFile:           tls.CertFile,
			KeyFile:            tls.KeyFile,
			ExclusiveRootPools: tls.Verify,
			InsecureSkipVerify: !tls.Verify,
		})
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConfig
	}

	scheme := "http"
	if tls.Enabled {
		scheme = "https"
	}

	opts := []client.Opt{
		client.WithAPIVersionNegotiation(),
		client.WithHTTPClient(httpClient),
		client.WithScheme(scheme),
	}
	if host != "" {
		normalizedHost, err := normalizeDockerHost(host, tls.Enabled)
		if err != nil {
			return nil, err
		}
		opts = append(opts, client.WithHost(normalizedHost))
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

func normalizeDockerHost(host string, tlsEnabled bool) (string, error) {
	parsed, err := url.Parse(host)
	if err != nil || parsed.Scheme == "" {
		return host, err
	}

	switch parsed.Scheme {
	case "http":
		if parsed.Host == "" {
			return host, nil
		}
		return "tcp://" + parsed.Host, nil
	case "https":
		if !tlsEnabled {
			return "", errors.New("https docker host requires TLS configuration")
		}
		if parsed.Host == "" {
			return host, nil
		}
		return "tcp://" + parsed.Host, nil
	default:
		if (parsed.Scheme == "unix" || parsed.Scheme == "npipe") && tlsEnabled {
			return "", errors.New("docker tls is not supported for unix or npipe hosts")
		}
		return host, nil
	}
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

// GetActualState retrieves the current state of services, optionally scoped to a stack.
func (c *DockerClient) GetActualState(ctx context.Context, stackName string) (*ActualState, error) {
	if c == nil || c.api == nil {
		return nil, errors.New("docker client is not initialized")
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	serviceFilters := filters.NewArgs()
	if stackName != "" {
		serviceFilters.Add("label", "com.docker.stack.namespace="+stackName)
	}

	services, err := c.api.ServiceList(ctx, dockertypes.ServiceListOptions{
		Filters: serviceFilters,
		Status:  true,
	})
	if err != nil {
		return nil, err
	}

	state := &ActualState{
		Services: make(map[string]ActualService, len(services)),
	}

	for _, service := range services {
		actualService, err := c.collectServiceState(ctx, service, stackName)
		if err != nil {
			return nil, err
		}
		state.Services[actualService.Name] = actualService
	}

	return state, nil
}

func (c *DockerClient) collectServiceState(ctx context.Context, service swarmtypes.Service, stackName string) (ActualService, error) {
	name := normalizeServiceName(service.Spec.Name, stackName)
	mode, desired := serviceModeAndReplicas(service)
	image := ""
	if service.Spec.TaskTemplate.ContainerSpec != nil {
		image = service.Spec.TaskTemplate.ContainerSpec.Image
	}

	// Docker API doesn't paginate; query per service to keep task lists bounded.
	taskFilters := filters.NewArgs(filters.Arg("service", service.ID))
	tasks, err := c.api.TaskList(ctx, dockertypes.TaskListOptions{Filters: taskFilters})
	if err != nil {
		return ActualService{}, err
	}

	runningReplicas, configs, secrets := summarizeTasks(tasks)

	return ActualService{
		Name:            name,
		Image:           image,
		Mode:            mode,
		DesiredReplicas: desired,
		RunningReplicas: runningReplicas,
		Configs:         configs,
		Secrets:         secrets,
	}, nil
}

func normalizeServiceName(name, stackName string) string {
	if stackName == "" {
		return name
	}
	prefix := stackName + "_"
	if strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix)
	}
	return name
}

func serviceModeAndReplicas(service swarmtypes.Service) (string, int) {
	if service.Spec.Mode.Replicated != nil {
		var replicas int
		replicasSet := false
		if service.Spec.Mode.Replicated.Replicas != nil {
			replicas = int(*service.Spec.Mode.Replicated.Replicas)
			replicasSet = true
		}
		if !replicasSet && service.ServiceStatus != nil {
			replicas = int(service.ServiceStatus.DesiredTasks)
		}
		return "replicated", replicas
	}
	if service.Spec.Mode.Global != nil {
		desired := 0
		if service.ServiceStatus != nil {
			desired = int(service.ServiceStatus.DesiredTasks)
		}
		return "global", desired
	}
	if service.Spec.Mode.ReplicatedJob != nil {
		desired := 0
		if service.ServiceStatus != nil {
			desired = int(service.ServiceStatus.DesiredTasks)
		}
		return "replicated-job", desired
	}
	if service.Spec.Mode.GlobalJob != nil {
		desired := 0
		if service.ServiceStatus != nil {
			desired = int(service.ServiceStatus.DesiredTasks)
		}
		return "global-job", desired
	}
	desired := 0
	if service.ServiceStatus != nil {
		desired = int(service.ServiceStatus.DesiredTasks)
	}
	return "unknown", desired
}

func summarizeTasks(tasks []swarmtypes.Task) (int, []string, []string) {
	running := 0
	configs := make(map[string]struct{})
	secrets := make(map[string]struct{})

	for _, task := range tasks {
		if task.Status.State != swarmtypes.TaskStateRunning {
			continue
		}
		running++

		spec := task.Spec.ContainerSpec
		if spec == nil {
			continue
		}

		for _, cfg := range spec.Configs {
			if cfg == nil || cfg.ConfigName == "" {
				continue
			}
			configs[cfg.ConfigName] = struct{}{}
		}
		for _, secret := range spec.Secrets {
			if secret == nil || secret.SecretName == "" {
				continue
			}
			secrets[secret.SecretName] = struct{}{}
		}
	}

	return running, normalizeNames(configs), normalizeNames(secrets)
}

func normalizeNames(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
