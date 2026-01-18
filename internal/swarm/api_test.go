package swarm

import (
	"context"
	"errors"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	swarmtypes "github.com/docker/docker/api/types/swarm"
)

// mockDockerAPI implements dockerAPI for testing.
type mockDockerAPI struct {
	pingFn        func(ctx context.Context) (dockertypes.Ping, error)
	serviceListFn func(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error)
	taskListFn    func(ctx context.Context, options dockertypes.TaskListOptions) ([]swarmtypes.Task, error)
	closeFn       func() error
}

func (m *mockDockerAPI) Ping(ctx context.Context) (dockertypes.Ping, error) {
	if m.pingFn != nil {
		return m.pingFn(ctx)
	}
	return dockertypes.Ping{}, nil
}

func (m *mockDockerAPI) ServiceList(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error) {
	if m.serviceListFn != nil {
		return m.serviceListFn(ctx, options)
	}
	return nil, nil
}

func (m *mockDockerAPI) TaskList(ctx context.Context, options dockertypes.TaskListOptions) ([]swarmtypes.Task, error) {
	if m.taskListFn != nil {
		return m.taskListFn(ctx, options)
	}
	return nil, nil
}

func (m *mockDockerAPI) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func TestDockerClient_Ping_Success(t *testing.T) {
	t.Parallel()

	mock := &mockDockerAPI{
		pingFn: func(ctx context.Context) (dockertypes.Ping, error) {
			return dockertypes.Ping{APIVersion: "1.41"}, nil
		},
	}

	client := &DockerClient{api: mock, timeout: 5 * time.Second}
	err := client.Ping(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerClient_Ping_Error(t *testing.T) {
	t.Parallel()

	mock := &mockDockerAPI{
		pingFn: func(ctx context.Context) (dockertypes.Ping, error) {
			return dockertypes.Ping{}, errors.New("connection refused")
		},
	}

	client := &DockerClient{api: mock, timeout: 5 * time.Second}
	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "connection refused" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDockerClient_GetActualState_Basic(t *testing.T) {
	t.Parallel()

	replicas := uint64(3)
	services := []swarmtypes.Service{
		{
			ID: "svc1",
			Spec: swarmtypes.ServiceSpec{
				Annotations: swarmtypes.Annotations{Name: "prod_web"},
				Mode: swarmtypes.ServiceMode{
					Replicated: &swarmtypes.ReplicatedService{Replicas: &replicas},
				},
				TaskTemplate: swarmtypes.TaskSpec{
					ContainerSpec: &swarmtypes.ContainerSpec{
						Image: "nginx:1.23@sha256:abc123",
					},
				},
			},
		},
	}

	tasks := []swarmtypes.Task{
		{
			ID:        "task1",
			ServiceID: "svc1",
			Status:    swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning},
			Spec: swarmtypes.TaskSpec{
				ContainerSpec: &swarmtypes.ContainerSpec{
					Configs: []*swarmtypes.ConfigReference{{ConfigName: "app_config_v1"}},
					Secrets: []*swarmtypes.SecretReference{{SecretName: "db_secret_v2"}},
				},
			},
		},
		{
			ID:        "task2",
			ServiceID: "svc1",
			Status:    swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning},
			Spec: swarmtypes.TaskSpec{
				ContainerSpec: &swarmtypes.ContainerSpec{
					Configs: []*swarmtypes.ConfigReference{{ConfigName: "app_config_v1"}},
					Secrets: []*swarmtypes.SecretReference{{SecretName: "db_secret_v2"}},
				},
			},
		},
		{
			ID:        "task3",
			ServiceID: "svc1",
			Status:    swarmtypes.TaskStatus{State: swarmtypes.TaskStateFailed},
			Spec: swarmtypes.TaskSpec{
				ContainerSpec: &swarmtypes.ContainerSpec{},
			},
		},
	}

	mock := &mockDockerAPI{
		serviceListFn: func(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error) {
			return services, nil
		},
		taskListFn: func(ctx context.Context, options dockertypes.TaskListOptions) ([]swarmtypes.Task, error) {
			return tasks, nil
		},
	}

	client := &DockerClient{api: mock, timeout: 5 * time.Second}
	state, err := client.GetActualState(context.Background(), "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(state.Services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(state.Services))
	}

	web, ok := state.Services["web"]
	if !ok {
		t.Fatal("expected web service")
	}

	if web.Image != "nginx:1.23@sha256:abc123" {
		t.Errorf("unexpected image: %q", web.Image)
	}
	if web.Mode != "replicated" {
		t.Errorf("unexpected mode: %q", web.Mode)
	}
	if web.DesiredReplicas != 3 {
		t.Errorf("expected 3 desired replicas, got %d", web.DesiredReplicas)
	}
	if web.RunningReplicas != 2 {
		t.Errorf("expected 2 running replicas, got %d", web.RunningReplicas)
	}
	if len(web.Configs) != 1 || web.Configs[0] != "app_config_v1" {
		t.Errorf("unexpected configs: %v", web.Configs)
	}
	if len(web.Secrets) != 1 || web.Secrets[0] != "db_secret_v2" {
		t.Errorf("unexpected secrets: %v", web.Secrets)
	}
}

func TestDockerClient_GetActualState_GlobalMode(t *testing.T) {
	t.Parallel()

	services := []swarmtypes.Service{
		{
			ID: "svc1",
			Spec: swarmtypes.ServiceSpec{
				Annotations: swarmtypes.Annotations{Name: "monitoring_agent"},
				Mode: swarmtypes.ServiceMode{
					Global: &swarmtypes.GlobalService{},
				},
				TaskTemplate: swarmtypes.TaskSpec{
					ContainerSpec: &swarmtypes.ContainerSpec{
						Image: "agent:latest",
					},
				},
			},
			ServiceStatus: &swarmtypes.ServiceStatus{
				DesiredTasks: 5,
				RunningTasks: 5,
			},
		},
	}

	tasks := []swarmtypes.Task{
		{ID: "t1", ServiceID: "svc1", Status: swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning}, Spec: swarmtypes.TaskSpec{ContainerSpec: &swarmtypes.ContainerSpec{}}},
		{ID: "t2", ServiceID: "svc1", Status: swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning}, Spec: swarmtypes.TaskSpec{ContainerSpec: &swarmtypes.ContainerSpec{}}},
		{ID: "t3", ServiceID: "svc1", Status: swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning}, Spec: swarmtypes.TaskSpec{ContainerSpec: &swarmtypes.ContainerSpec{}}},
		{ID: "t4", ServiceID: "svc1", Status: swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning}, Spec: swarmtypes.TaskSpec{ContainerSpec: &swarmtypes.ContainerSpec{}}},
		{ID: "t5", ServiceID: "svc1", Status: swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning}, Spec: swarmtypes.TaskSpec{ContainerSpec: &swarmtypes.ContainerSpec{}}},
	}

	mock := &mockDockerAPI{
		serviceListFn: func(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error) {
			return services, nil
		},
		taskListFn: func(ctx context.Context, options dockertypes.TaskListOptions) ([]swarmtypes.Task, error) {
			return tasks, nil
		},
	}

	client := &DockerClient{api: mock, timeout: 5 * time.Second}
	state, err := client.GetActualState(context.Background(), "monitoring")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	agent := state.Services["agent"]
	if agent.Mode != "global" {
		t.Errorf("expected global mode, got %q", agent.Mode)
	}
	if agent.DesiredReplicas != 5 {
		t.Errorf("expected 5 desired (from ServiceStatus), got %d", agent.DesiredReplicas)
	}
	if agent.RunningReplicas != 5 {
		t.Errorf("expected 5 running replicas, got %d", agent.RunningReplicas)
	}
}

func TestDockerClient_GetActualState_ServiceListError(t *testing.T) {
	t.Parallel()

	mock := &mockDockerAPI{
		serviceListFn: func(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error) {
			return nil, errors.New("docker api error")
		},
	}

	client := &DockerClient{api: mock, timeout: 5 * time.Second}
	_, err := client.GetActualState(context.Background(), "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDockerClient_GetActualState_EmptyCluster(t *testing.T) {
	t.Parallel()

	mock := &mockDockerAPI{
		serviceListFn: func(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error) {
			return []swarmtypes.Service{}, nil
		},
	}

	client := &DockerClient{api: mock, timeout: 5 * time.Second}
	state, err := client.GetActualState(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.Services) != 0 {
		t.Errorf("expected empty services map, got %d services", len(state.Services))
	}
}

func TestDockerClient_GetActualState_StackFilter(t *testing.T) {
	t.Parallel()

	var capturedFilter string
	mock := &mockDockerAPI{
		serviceListFn: func(ctx context.Context, options dockertypes.ServiceListOptions) ([]swarmtypes.Service, error) {
			if options.Filters.Len() > 0 {
				capturedFilter = options.Filters.Get("label")[0]
			}
			return []swarmtypes.Service{}, nil
		},
	}

	client := &DockerClient{api: mock, timeout: 5 * time.Second}
	_, _ = client.GetActualState(context.Background(), "mystack")

	expected := "com.docker.stack.namespace=mystack"
	if capturedFilter != expected {
		t.Errorf("expected filter %q, got %q", expected, capturedFilter)
	}
}

func TestDockerClient_Close(t *testing.T) {
	t.Parallel()

	closed := false
	mock := &mockDockerAPI{
		closeFn: func() error {
			closed = true
			return nil
		},
	}

	client := &DockerClient{api: mock, timeout: 5 * time.Second}
	err := client.Close()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !closed {
		t.Error("expected Close to be called")
	}
}

func TestDockerClient_Close_NilClient(t *testing.T) {
	t.Parallel()

	var client *DockerClient
	err := client.Close()
	if err != nil {
		t.Fatalf("unexpected error for nil client: %v", err)
	}
}
