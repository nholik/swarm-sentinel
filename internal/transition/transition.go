package transition

import (
	"sort"

	"github.com/nholik/swarm-sentinel/internal/health"
	"github.com/nholik/swarm-sentinel/internal/state"
)

// ReplicaChange captures replica count changes between snapshots.
type ReplicaChange struct {
	PreviousDesired int
	CurrentDesired  int
	PreviousRunning int
	CurrentRunning  int
	DesiredDelta    int
	RunningDelta    int
}

// ImageChange captures image details during a transition.
type ImageChange struct {
	PreviousDesired string
	CurrentDesired  string
	PreviousActual  string
	CurrentActual   string
}

// ServiceTransition captures a status transition with details.
type ServiceTransition struct {
	Name           string
	PreviousStatus health.ServiceStatus
	CurrentStatus  health.ServiceStatus
	Reasons        []string
	Drift          []health.DriftDetail
	ReplicaChange  *ReplicaChange
	ImageChange    *ImageChange
}

// DetectServiceTransitions compares a previous snapshot with current health and emits transitions.
func DetectServiceTransitions(prev *state.StackSnapshot, current health.StackHealth) []ServiceTransition {
	prevServices := map[string]health.ServiceHealth{}
	if prev != nil && prev.Services != nil {
		prevServices = prev.Services
	}
	firstRun := prev == nil || len(prevServices) == 0

	transitions := make([]ServiceTransition, 0)
	for name, currentService := range current.Services {
		prevService, hadPrev := prevServices[name]
		prevStatus := prevService.Status
		if prevService.LastNotifiedStatus != "" {
			prevStatus = prevService.LastNotifiedStatus
		}

		if firstRun {
			if currentService.Status == health.StatusOK {
				continue
			}
		} else if hadPrev {
			if prevStatus == currentService.Status {
				continue
			}
		} else if currentService.Status == health.StatusOK {
			continue
		}

		transitions = append(transitions, ServiceTransition{
			Name:           name,
			PreviousStatus: prevStatus,
			CurrentStatus:  currentService.Status,
			Reasons:        append([]string(nil), currentService.Reasons...),
			Drift:          append([]health.DriftDetail(nil), currentService.Drift...),
			ReplicaChange:  buildReplicaChange(prevService, currentService, hadPrev),
			ImageChange:    buildImageChange(prevService, currentService, hadPrev),
		})
	}

	// Sort by service name for deterministic output
	sort.Slice(transitions, func(i, j int) bool {
		return transitions[i].Name < transitions[j].Name
	})

	return transitions
}

func buildReplicaChange(prev health.ServiceHealth, current health.ServiceHealth, hadPrev bool) *ReplicaChange {
	// Skip if new service with zero replicas (not meaningful change info)
	if !hadPrev && current.DesiredReplicas == 0 && current.RunningReplicas == 0 {
		return nil
	}
	return &ReplicaChange{
		PreviousDesired: prev.DesiredReplicas,
		CurrentDesired:  current.DesiredReplicas,
		PreviousRunning: prev.RunningReplicas,
		CurrentRunning:  current.RunningReplicas,
		DesiredDelta:    current.DesiredReplicas - prev.DesiredReplicas,
		RunningDelta:    current.RunningReplicas - prev.RunningReplicas,
	}
}

func buildImageChange(prev health.ServiceHealth, current health.ServiceHealth, hadPrev bool) *ImageChange {
	// Skip if new service with no image info (not meaningful change info)
	if !hadPrev && current.DesiredImage == "" && current.ActualImage == "" {
		return nil
	}
	return &ImageChange{
		PreviousDesired: prev.DesiredImage,
		CurrentDesired:  current.DesiredImage,
		PreviousActual:  prev.ActualImage,
		CurrentActual:   current.ActualImage,
	}
}
