package transition

import (
	"testing"

	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/nholik/swarm-sentinel/internal/state"
)

func TestDetectServiceTransitions_FirstRun(t *testing.T) {
	current := health.StackHealth{
		Status: health.StatusDegraded,
		Services: map[string]health.ServiceHealth{
			"ok": {
				Name:   "ok",
				Status: health.StatusOK,
			},
			"bad": {
				Name:            "bad",
				Status:          health.StatusFailed,
				DesiredReplicas: 2,
				RunningReplicas: 0,
				Reasons:         []string{"no running replicas"},
			},
		},
	}

	transitions := DetectServiceTransitions(nil, current)

	if len(transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(transitions))
	}
	if transitions[0].Name != "bad" {
		t.Fatalf("expected transition for bad, got %s", transitions[0].Name)
	}
	if transitions[0].CurrentStatus != health.StatusFailed {
		t.Fatalf("expected failed status, got %s", transitions[0].CurrentStatus)
	}
	if transitions[0].ReplicaChange == nil || transitions[0].ReplicaChange.CurrentDesired != 2 {
		t.Fatalf("expected replica change details, got %+v", transitions[0].ReplicaChange)
	}
}

func TestDetectServiceTransitions_NoOp(t *testing.T) {
	prev := &state.StackSnapshot{
		Services: map[string]health.ServiceHealth{
			"api": {
				Name:   "api",
				Status: health.StatusDegraded,
			},
		},
	}
	current := health.StackHealth{
		Status: health.StatusDegraded,
		Services: map[string]health.ServiceHealth{
			"api": {
				Name:   "api",
				Status: health.StatusDegraded,
			},
		},
	}

	transitions := DetectServiceTransitions(prev, current)
	if len(transitions) != 0 {
		t.Fatalf("expected no transitions, got %d", len(transitions))
	}
}

func TestDetectServiceTransitions_Mixed(t *testing.T) {
	prev := &state.StackSnapshot{
		Services: map[string]health.ServiceHealth{
			"web": {
				Name:            "web",
				Status:          health.StatusOK,
				DesiredReplicas: 2,
				RunningReplicas: 2,
				DesiredImage:    "nginx:1.23",
				ActualImage:     "nginx:1.23",
			},
			"api": {
				Name:            "api",
				Status:          health.StatusFailed,
				DesiredReplicas: 2,
				RunningReplicas: 0,
			},
			"cache": {
				Name:   "cache",
				Status: health.StatusDegraded,
			},
		},
	}
	current := health.StackHealth{
		Status: health.StatusDegraded,
		Services: map[string]health.ServiceHealth{
			"web": {
				Name:            "web",
				Status:          health.StatusDegraded,
				DesiredReplicas: 2,
				RunningReplicas: 1,
				DesiredImage:    "nginx:1.23",
				ActualImage:     "nginx:1.23",
				Reasons:         []string{"replicas running 1/2"},
			},
			"api": {
				Name:            "api",
				Status:          health.StatusFailed,
				DesiredReplicas: 2,
				RunningReplicas: 0,
			},
			"cache": {
				Name:   "cache",
				Status: health.StatusOK,
			},
			"worker": {
				Name:   "worker",
				Status: health.StatusFailed,
				Drift: []health.DriftDetail{
					{Kind: health.DriftMissing, Resource: "secret", Name: "token"},
				},
			},
		},
	}

	transitions := DetectServiceTransitions(prev, current)
	if len(transitions) != 3 {
		t.Fatalf("expected 3 transitions, got %d", len(transitions))
	}

	found := map[string]ServiceTransition{}
	for _, transition := range transitions {
		found[transition.Name] = transition
	}

	web := found["web"]
	if web.CurrentStatus != health.StatusDegraded || web.PreviousStatus != health.StatusOK {
		t.Fatalf("unexpected web transition: %+v", web)
	}
	if web.ReplicaChange == nil || web.ReplicaChange.RunningDelta != -1 {
		t.Fatalf("expected replica delta, got %+v", web.ReplicaChange)
	}

	cache := found["cache"]
	if cache.CurrentStatus != health.StatusOK || cache.PreviousStatus != health.StatusDegraded {
		t.Fatalf("unexpected cache transition: %+v", cache)
	}

	worker := found["worker"]
	if worker.CurrentStatus != health.StatusFailed || worker.PreviousStatus != "" {
		t.Fatalf("unexpected worker transition: %+v", worker)
	}
	if len(worker.Drift) != 1 || worker.Drift[0].Name != "token" {
		t.Fatalf("expected drift details, got %+v", worker.Drift)
	}
}
