package compose

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

const (
	defaultDeployMode   = "replicated"
	globalDeployMode    = "global"
	defaultServiceScale = 1
)

// DesiredState represents the normalized desired state from a compose file.
type DesiredState struct {
	Services map[string]DesiredService
}

// DesiredService captures the fields we track for a service.
type DesiredService struct {
	Image    string
	Mode     string
	Replicas int
	Configs  []string
	Secrets  []string
}

// ParseDesiredState parses compose content into a normalized desired state model.
func ParseDesiredState(ctx context.Context, body []byte) (DesiredState, error) {
	if len(body) == 0 {
		return DesiredState{}, errors.New("compose body is empty")
	}

	details := types.ConfigDetails{
		WorkingDir: ".",
		ConfigFiles: []types.ConfigFile{
			{
				Filename: "compose.yml",
				Content:  body,
			},
		},
		Environment: types.Mapping{},
	}

	project, err := loader.LoadWithContext(ctx, details, func(opts *loader.Options) {
		opts.SetProjectName("swarm-sentinel", false)
	})
	if err != nil {
		return DesiredState{}, fmt.Errorf("load compose: %w", err)
	}
	if len(project.Services) == 0 {
		return DesiredState{}, errors.New("compose has no services")
	}

	state := DesiredState{
		Services: make(map[string]DesiredService, len(project.Services)),
	}

	for name, service := range project.Services {
		if service.Image == "" {
			return DesiredState{}, fmt.Errorf("service %q missing image", name)
		}

		mode := defaultDeployMode
		if service.Deploy != nil && service.Deploy.Mode != "" {
			mode = service.Deploy.Mode
		}

		replicas := defaultServiceScale
		if mode == globalDeployMode {
			// Global mode replicas are set to 0 at parse time because the actual
			// count depends on the number of nodes in the cluster, which is only
			// known at runtime. The actual state uses ServiceStatus.DesiredTasks.
			replicas = 0
		} else if service.Deploy != nil && service.Deploy.Replicas != nil {
			replicas = *service.Deploy.Replicas
		} else if service.Scale != nil {
			replicas = *service.Scale
		}

		configs, err := resolveConfigNames(service.Configs, project.Configs)
		if err != nil {
			return DesiredState{}, fmt.Errorf("service %q configs: %w", name, err)
		}

		secrets, err := resolveSecretNames(service.Secrets, project.Secrets)
		if err != nil {
			return DesiredState{}, fmt.Errorf("service %q secrets: %w", name, err)
		}

		state.Services[name] = DesiredService{
			Image:    service.Image,
			Mode:     mode,
			Replicas: replicas,
			Configs:  configs,
			Secrets:  secrets,
		}
	}

	return state, nil
}

func resolveConfigNames(refs []types.ServiceConfigObjConfig, configs types.Configs) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		source := ref.Source
		if source == "" {
			return nil, errors.New("config reference missing source")
		}
		cfg, ok := configs[source]
		if !ok {
			return nil, fmt.Errorf("undefined config %q", source)
		}
		name := cfg.Name
		if name == "" {
			name = source
		}
		names = append(names, name)
	}

	return normalizeNames(names), nil
}

func resolveSecretNames(refs []types.ServiceSecretConfig, secrets types.Secrets) ([]string, error) {
	if len(refs) == 0 {
		return nil, nil
	}

	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		source := ref.Source
		if source == "" {
			return nil, errors.New("secret reference missing source")
		}
		secret, ok := secrets[source]
		if !ok {
			return nil, fmt.Errorf("undefined secret %q", source)
		}
		name := secret.Name
		if name == "" {
			name = source
		}
		names = append(names, name)
	}

	return normalizeNames(names), nil
}

func normalizeNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	result := make([]string, 0, len(values))
	var last string
	for _, value := range values {
		if value == last {
			continue
		}
		result = append(result, value)
		last = value
	}
	return result
}
