package health

import (
	"fmt"
	"sort"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/swarm"
)

// EvaluateStackHealth compares desired and actual state to compute health.
func EvaluateStackHealth(desired compose.DesiredState, actual *swarm.ActualState, stackScoped bool) StackHealth {
	if actual == nil {
		actual = &swarm.ActualState{Services: map[string]swarm.ActualService{}}
	}
	if actual.Services == nil {
		actual.Services = map[string]swarm.ActualService{}
	}

	result := StackHealth{
		Status:   StatusOK,
		Services: make(map[string]ServiceHealth),
	}

	for name, desiredService := range desired.Services {
		actualService, ok := actual.Services[name]
		if !ok {
			health := ServiceHealth{
				Name:            name,
				Status:          StatusFailed,
				Reasons:         []string{"missing service"},
				DesiredImage:    swarm.NormalizeImage(desiredService.Image),
				DesiredReplicas: desiredService.Replicas,
			}
			result.Services[name] = health
			result.Status = worsenStatus(result.Status, health.Status)
			continue
		}
		health := evaluateService(name, desiredService, actualService)
		result.Services[name] = health
		result.Status = worsenStatus(result.Status, health.Status)
	}

	if stackScoped {
		for name, actualService := range actual.Services {
			if _, ok := desired.Services[name]; ok {
				continue
			}
			health := ServiceHealth{
				Name:            name,
				Status:          StatusDegraded,
				Reasons:         []string{"extra service"},
				ActualImage:     swarm.NormalizeImage(actualService.Image),
				DesiredReplicas: actualService.DesiredReplicas,
				RunningReplicas: actualService.RunningReplicas,
				Drift: []DriftDetail{
					{
						Kind:     DriftExtraService,
						Resource: "service",
						Name:     actualService.Name,
					},
				},
			}
			result.Services[name] = health
			result.Status = worsenStatus(result.Status, health.Status)
		}
	}

	return result
}

func evaluateService(name string, desired compose.DesiredService, actual swarm.ActualService) ServiceHealth {
	health := ServiceHealth{
		Name:   name,
		Status: StatusOK,
	}

	desiredImage := swarm.NormalizeImage(desired.Image)
	actualImage := swarm.NormalizeImage(actual.Image)
	health.DesiredImage = desiredImage
	health.ActualImage = actualImage
	if desiredImage != actualImage {
		health.Status = worsenStatus(health.Status, StatusDegraded)
		health.Reasons = append(health.Reasons, fmt.Sprintf("image mismatch: want %s got %s", desiredImage, actualImage))
	}

	desiredReplicas := desired.Replicas
	if desired.Mode == "global" {
		desiredReplicas = actual.DesiredReplicas
	}
	health.DesiredReplicas = desiredReplicas
	health.RunningReplicas = actual.RunningReplicas

	if desiredReplicas > 0 {
		switch {
		case actual.RunningReplicas == 0:
			health.Status = worsenStatus(health.Status, StatusFailed)
			health.Reasons = append(health.Reasons, fmt.Sprintf("no running replicas (desired %d)", desiredReplicas))
		case actual.RunningReplicas < desiredReplicas:
			health.Status = worsenStatus(health.Status, StatusDegraded)
			health.Reasons = append(health.Reasons, fmt.Sprintf("replicas running %d/%d", actual.RunningReplicas, desiredReplicas))
		case actual.RunningReplicas > desiredReplicas:
			health.Status = worsenStatus(health.Status, StatusDegraded)
			health.Reasons = append(health.Reasons, fmt.Sprintf("replicas running %d/%d", actual.RunningReplicas, desiredReplicas))
		}
	}

	health.Reasons, health.Drift = applyDrift(health.Reasons, health.Drift, "config", desired.Configs, actual.Configs)
	health.Reasons, health.Drift = applyDrift(health.Reasons, health.Drift, "secret", desired.Secrets, actual.Secrets)

	for _, drift := range health.Drift {
		switch drift.Kind {
		case DriftMissing:
			health.Status = worsenStatus(health.Status, StatusFailed)
		case DriftExtra:
			health.Status = worsenStatus(health.Status, StatusDegraded)
		}
	}

	return health
}

func applyDrift(reasons []string, drift []DriftDetail, resource string, desired, actual []string) ([]string, []DriftDetail) {
	missing, extra := diffNames(desired, actual)
	for _, name := range missing {
		reasons = append(reasons, fmt.Sprintf("missing %s: %s", resource, name))
		drift = append(drift, DriftDetail{
			Kind:     DriftMissing,
			Resource: resource,
			Name:     name,
		})
	}
	for _, name := range extra {
		reasons = append(reasons, fmt.Sprintf("extra %s: %s", resource, name))
		drift = append(drift, DriftDetail{
			Kind:     DriftExtra,
			Resource: resource,
			Name:     name,
		})
	}
	return reasons, drift
}

func diffNames(desired, actual []string) ([]string, []string) {
	if len(desired) == 0 && len(actual) == 0 {
		return nil, nil
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, name := range actual {
		actualSet[name] = struct{}{}
	}
	missing := make([]string, 0)
	for _, name := range desired {
		if _, ok := actualSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	desiredSet := make(map[string]struct{}, len(desired))
	for _, name := range desired {
		desiredSet[name] = struct{}{}
	}
	extra := make([]string, 0)
	for _, name := range actual {
		if _, ok := desiredSet[name]; !ok {
			extra = append(extra, name)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

func worsenStatus(current, next ServiceStatus) ServiceStatus {
	if severity(next) > severity(current) {
		return next
	}
	return current
}

func severity(status ServiceStatus) int {
	switch status {
	case StatusFailed:
		return 2
	case StatusDegraded:
		return 1
	default:
		return 0
	}
}
