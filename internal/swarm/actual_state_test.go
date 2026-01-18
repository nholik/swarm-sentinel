package swarm

import (
	"reflect"
	"testing"

	swarmtypes "github.com/docker/docker/api/types/swarm"
)

func TestNormalizeServiceName(t *testing.T) {
	t.Parallel()

	if got := normalizeServiceName("prod_web", "prod"); got != "web" {
		t.Fatalf("expected trimmed name, got %q", got)
	}
	if got := normalizeServiceName("web", "prod"); got != "web" {
		t.Fatalf("expected original name when prefix missing, got %q", got)
	}
	if got := normalizeServiceName("web", ""); got != "web" {
		t.Fatalf("expected original name when stack empty, got %q", got)
	}
}

func TestServiceModeAndReplicas_Replicated(t *testing.T) {
	t.Parallel()

	replicas := uint64(3)
	service := swarmtypes.Service{
		Spec: swarmtypes.ServiceSpec{
			Annotations: swarmtypes.Annotations{Name: "web"},
			Mode: swarmtypes.ServiceMode{
				Replicated: &swarmtypes.ReplicatedService{
					Replicas: &replicas,
				},
			},
		},
	}

	mode, desired := serviceModeAndReplicas(service)
	if mode != "replicated" {
		t.Fatalf("expected replicated mode, got %q", mode)
	}
	if desired != 3 {
		t.Fatalf("expected 3 replicas, got %d", desired)
	}
}

func TestServiceModeAndReplicas_ReplicatedFallback(t *testing.T) {
	t.Parallel()

	service := swarmtypes.Service{
		Spec: swarmtypes.ServiceSpec{
			Annotations: swarmtypes.Annotations{Name: "web"},
			Mode: swarmtypes.ServiceMode{
				Replicated: &swarmtypes.ReplicatedService{},
			},
		},
		ServiceStatus: &swarmtypes.ServiceStatus{
			DesiredTasks: 5,
		},
	}

	mode, desired := serviceModeAndReplicas(service)
	if mode != "replicated" {
		t.Fatalf("expected replicated mode, got %q", mode)
	}
	if desired != 5 {
		t.Fatalf("expected 5 replicas, got %d", desired)
	}
}

func TestServiceModeAndReplicas_Global(t *testing.T) {
	t.Parallel()

	service := swarmtypes.Service{
		Spec: swarmtypes.ServiceSpec{
			Annotations: swarmtypes.Annotations{Name: "web"},
			Mode: swarmtypes.ServiceMode{
				Global: &swarmtypes.GlobalService{},
			},
		},
		ServiceStatus: &swarmtypes.ServiceStatus{
			DesiredTasks: 4,
		},
	}

	mode, desired := serviceModeAndReplicas(service)
	if mode != "global" {
		t.Fatalf("expected global mode, got %q", mode)
	}
	if desired != 4 {
		t.Fatalf("expected 4 desired tasks, got %d", desired)
	}
}

func TestSummarizeTasks_RunningOnly(t *testing.T) {
	t.Parallel()

	tasks := []swarmtypes.Task{
		{
			Status: swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning},
			Spec: swarmtypes.TaskSpec{
				ContainerSpec: &swarmtypes.ContainerSpec{
					Configs: []*swarmtypes.ConfigReference{
						{ConfigName: "app_config_v2"},
						nil,
					},
					Secrets: []*swarmtypes.SecretReference{
						{SecretName: "db_secret_v1"},
					},
				},
			},
		},
		{
			Status: swarmtypes.TaskStatus{State: swarmtypes.TaskStateRunning},
			Spec: swarmtypes.TaskSpec{
				ContainerSpec: &swarmtypes.ContainerSpec{
					Configs: []*swarmtypes.ConfigReference{
						{ConfigName: "app_config_v2"},
						{ConfigName: "other_config_v1"},
					},
					Secrets: []*swarmtypes.SecretReference{
						{SecretName: "db_secret_v1"},
						{SecretName: "api_secret_v3"},
					},
				},
			},
		},
		{
			Status: swarmtypes.TaskStatus{State: swarmtypes.TaskStateFailed},
			Spec: swarmtypes.TaskSpec{
				ContainerSpec: &swarmtypes.ContainerSpec{
					Configs: []*swarmtypes.ConfigReference{
						{ConfigName: "ignored_config"},
					},
					Secrets: []*swarmtypes.SecretReference{
						{SecretName: "ignored_secret"},
					},
				},
			},
		},
	}

	running, configs, secrets := summarizeTasks(tasks)
	if running != 2 {
		t.Fatalf("expected 2 running tasks, got %d", running)
	}
	if !reflect.DeepEqual(configs, []string{"app_config_v2", "other_config_v1"}) {
		t.Fatalf("unexpected configs: %+v", configs)
	}
	if !reflect.DeepEqual(secrets, []string{"api_secret_v3", "db_secret_v1"}) {
		t.Fatalf("unexpected secrets: %+v", secrets)
	}
}
