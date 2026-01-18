package transition

import (
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

		if firstRun {
			if currentService.Status == health.StatusOK {
				continue
			}
		} else if hadPrev {
			if prevService.Status == currentService.Status {
				continue
			}
		} else if currentService.Status == health.StatusOK {
			continue
		}

		transitions = append(transitions, ServiceTransition{
			Name:           name,
			PreviousStatus: prevService.Status,
			CurrentStatus:  currentService.Status,
			Reasons:        append([]string(nil), currentService.Reasons...),
			Drift:          append([]health.DriftDetail(nil), currentService.Drift...),
			ReplicaChange:  buildReplicaChange(prevService, currentService, hadPrev),
			ImageChange:    buildImageChange(prevService, currentService, hadPrev),
		})
	}

	return transitions
}

func buildReplicaChange(prev health.ServiceHealth, current health.ServiceHealth, hadPrev bool) *ReplicaChange {
	if !hadPrev && current.DesiredReplicas == 0 && current.RunningReplicas == 0 {
		return nil
	}
	if hadPrev || current.DesiredReplicas != 0 || current.RunningReplicas != 0 {
		return &ReplicaChange{
			PreviousDesired: prev.DesiredReplicas,
			CurrentDesired:  current.DesiredReplicas,
			PreviousRunning: prev.RunningReplicas,
			CurrentRunning:  current.RunningReplicas,
			DesiredDelta:    current.DesiredReplicas - prev.DesiredReplicas,
			RunningDelta:    current.RunningReplicas - prev.RunningReplicas,
		}
	}
	return nil
}

func buildImageChange(prev health.ServiceHealth, current health.ServiceHealth, hadPrev bool) *ImageChange {
	if !hadPrev && current.DesiredImage == "" && current.ActualImage == "" {
		return nil
	}
	if hadPrev || current.DesiredImage != "" || current.ActualImage != "" {
		return &ImageChange{
			PreviousDesired: prev.DesiredImage,
			CurrentDesired:  current.DesiredImage,
			PreviousActual:  prev.ActualImage,
			CurrentActual:   current.ActualImage,
		}
	}
	return nil
}
