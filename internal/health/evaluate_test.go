package health

import (
	"strings"
	"testing"

	"github.com/nholik/swarm-sentinel/internal/compose"
	"github.com/nholik/swarm-sentinel/internal/swarm"
)

func TestEvaluateStackHealth_MissingService(t *testing.T) {
	desired := compose.DesiredState{
		Services: map[string]compose.DesiredService{
			"api": {Image: "app:v1", Mode: "replicated", Replicas: 1},
		},
	}
	actual := &swarm.ActualState{Services: map[string]swarm.ActualService{}}

	health := EvaluateStackHealth(desired, actual, true)

	serviceHealth, ok := health.Services["api"]
	if !ok {
		t.Fatalf("expected service health for api")
	}
	if serviceHealth.Status != StatusFailed {
		t.Fatalf("expected failed status, got %s", serviceHealth.Status)
	}
	if len(serviceHealth.Reasons) == 0 || serviceHealth.Reasons[0] != "missing service" {
		t.Fatalf("expected missing service reason, got %v", serviceHealth.Reasons)
	}
	if health.Status != StatusFailed {
		t.Fatalf("expected stack status failed, got %s", health.Status)
	}
}

func TestEvaluateStackHealth_ExtraServiceStackScoped(t *testing.T) {
	desired := compose.DesiredState{Services: map[string]compose.DesiredService{}}
	actual := &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"extra": {Name: "extra", Image: "app:v1"},
		},
	}

	health := EvaluateStackHealth(desired, actual, true)

	serviceHealth, ok := health.Services["extra"]
	if !ok {
		t.Fatalf("expected service health for extra")
	}
	if serviceHealth.Status != StatusDegraded {
		t.Fatalf("expected degraded status, got %s", serviceHealth.Status)
	}
	if !hasDrift(serviceHealth.Drift, DriftExtraService, "service", "extra") {
		t.Fatalf("expected extra service drift detail, got %v", serviceHealth.Drift)
	}
	if health.Status != StatusDegraded {
		t.Fatalf("expected stack status degraded, got %s", health.Status)
	}
}

func TestEvaluateStackHealth_ExtraServiceIgnoredWhenNotScoped(t *testing.T) {
	desired := compose.DesiredState{Services: map[string]compose.DesiredService{}}
	actual := &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"extra": {Name: "extra", Image: "app:v1"},
		},
	}

	health := EvaluateStackHealth(desired, actual, false)

	if len(health.Services) != 0 {
		t.Fatalf("expected no service health entries, got %v", health.Services)
	}
	if health.Status != StatusOK {
		t.Fatalf("expected stack status ok, got %s", health.Status)
	}
}

func TestEvaluateStackHealth_ReplicaRules(t *testing.T) {
	cases := []struct {
		name           string
		running        int
		desired        int
		expectedStatus ServiceStatus
		expectedReason string
	}{
		{
			name:           "zero_running_failed",
			running:        0,
			desired:        2,
			expectedStatus: StatusFailed,
			expectedReason: "no running replicas",
		},
		{
			name:           "less_running_degraded",
			running:        1,
			desired:        3,
			expectedStatus: StatusDegraded,
			expectedReason: "replicas running 1/3",
		},
		{
			name:           "more_running_degraded",
			running:        4,
			desired:        3,
			expectedStatus: StatusDegraded,
			expectedReason: "replicas running 4/3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desired := compose.DesiredState{
				Services: map[string]compose.DesiredService{
					"api": {Image: "app:v1", Mode: "replicated", Replicas: tc.desired},
				},
			}
			actual := &swarm.ActualState{
				Services: map[string]swarm.ActualService{
					"api": {Name: "api", Image: "app:v1", RunningReplicas: tc.running},
				},
			}

			health := EvaluateStackHealth(desired, actual, true)
			serviceHealth := health.Services["api"]

			if serviceHealth.Status != tc.expectedStatus {
				t.Fatalf("expected %s status, got %s", tc.expectedStatus, serviceHealth.Status)
			}
			if !containsReason(serviceHealth.Reasons, tc.expectedReason) {
				t.Fatalf("expected reason %q, got %v", tc.expectedReason, serviceHealth.Reasons)
			}
		})
	}
}

func TestEvaluateStackHealth_GlobalModeReplicas(t *testing.T) {
	desired := compose.DesiredState{
		Services: map[string]compose.DesiredService{
			"agent": {Image: "agent:v1", Mode: "global", Replicas: 0},
		},
	}
	actual := &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"agent": {
				Name:            "agent",
				Image:           "agent:v1",
				DesiredReplicas: 3,
				RunningReplicas: 2,
			},
		},
	}

	health := EvaluateStackHealth(desired, actual, true)
	serviceHealth := health.Services["agent"]

	if serviceHealth.Status != StatusDegraded {
		t.Fatalf("expected degraded status, got %s", serviceHealth.Status)
	}
	if !containsReason(serviceHealth.Reasons, "replicas running 2/3") {
		t.Fatalf("expected global replica reason, got %v", serviceHealth.Reasons)
	}
}

func TestEvaluateStackHealth_ImageMismatch(t *testing.T) {
	desired := compose.DesiredState{
		Services: map[string]compose.DesiredService{
			"web": {Image: "nginx:1.23", Mode: "replicated", Replicas: 1},
		},
	}
	actual := &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"web": {Name: "web", Image: "nginx:1.24@sha256:abc", RunningReplicas: 1},
		},
	}

	health := EvaluateStackHealth(desired, actual, true)
	serviceHealth := health.Services["web"]

	if serviceHealth.Status != StatusDegraded {
		t.Fatalf("expected degraded status, got %s", serviceHealth.Status)
	}
	if !containsReason(serviceHealth.Reasons, "image mismatch") {
		t.Fatalf("expected image mismatch reason, got %v", serviceHealth.Reasons)
	}
}

func TestEvaluateStackHealth_ConfigSecretDrift(t *testing.T) {
	desired := compose.DesiredState{
		Services: map[string]compose.DesiredService{
			"api": {
				Image:   "app:v1",
				Mode:    "replicated",
				Configs: []string{"cfg1"},
				Secrets: []string{"sec1"},
			},
		},
	}
	actual := &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"api": {
				Name:    "api",
				Image:   "app:v1",
				Configs: []string{},
				Secrets: []string{"sec1", "sec2"},
			},
		},
	}

	health := EvaluateStackHealth(desired, actual, true)
	serviceHealth := health.Services["api"]

	if serviceHealth.Status != StatusFailed {
		t.Fatalf("expected failed status, got %s", serviceHealth.Status)
	}
	if !hasDrift(serviceHealth.Drift, DriftMissing, "config", "cfg1") {
		t.Fatalf("expected missing config drift, got %v", serviceHealth.Drift)
	}
	if !hasDrift(serviceHealth.Drift, DriftExtra, "secret", "sec2") {
		t.Fatalf("expected extra secret drift, got %v", serviceHealth.Drift)
	}
	if !containsReason(serviceHealth.Reasons, "missing config: cfg1") {
		t.Fatalf("expected missing config reason, got %v", serviceHealth.Reasons)
	}
	if !containsReason(serviceHealth.Reasons, "extra secret: sec2") {
		t.Fatalf("expected extra secret reason, got %v", serviceHealth.Reasons)
	}
}

func TestEvaluateStackHealth_UpdateInProgressSuppressesReplicaAlerts(t *testing.T) {
	desired := compose.DesiredState{
		Services: map[string]compose.DesiredService{
			"api": {Image: "app:v1", Mode: "replicated", Replicas: 3},
		},
	}
	actual := &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"api": {
				Name:            "api",
				Image:           "app:v1",
				DesiredReplicas: 3,
				RunningReplicas: 1,
				UpdateState:     "updating",
			},
		},
	}

	health := EvaluateStackHealth(desired, actual, true)
	serviceHealth := health.Services["api"]

	if serviceHealth.Status != StatusOK {
		t.Fatalf("expected ok status, got %s", serviceHealth.Status)
	}
	if len(serviceHealth.Reasons) != 0 {
		t.Fatalf("expected no reasons, got %v", serviceHealth.Reasons)
	}
}

func TestEvaluateStackHealth_UpdateInProgressStillAlertsOnZeroReplicas(t *testing.T) {
	desired := compose.DesiredState{
		Services: map[string]compose.DesiredService{
			"api": {Image: "app:v1", Mode: "replicated", Replicas: 3},
		},
	}
	actual := &swarm.ActualState{
		Services: map[string]swarm.ActualService{
			"api": {
				Name:            "api",
				Image:           "app:v1",
				DesiredReplicas: 3,
				RunningReplicas: 0,
				UpdateState:     "rollback_started",
			},
		},
	}

	health := EvaluateStackHealth(desired, actual, true)
	serviceHealth := health.Services["api"]

	if serviceHealth.Status != StatusFailed {
		t.Fatalf("expected failed status, got %s", serviceHealth.Status)
	}
	if !containsReason(serviceHealth.Reasons, "no running replicas") {
		t.Fatalf("expected replica failure reason, got %v", serviceHealth.Reasons)
	}
}

func containsReason(reasons []string, value string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, value) {
			return true
		}
	}
	return false
}

func hasDrift(details []DriftDetail, kind DriftKind, resource, name string) bool {
	for _, detail := range details {
		if detail.Kind == kind && detail.Resource == resource && detail.Name == name {
			return true
		}
	}
	return false
}
