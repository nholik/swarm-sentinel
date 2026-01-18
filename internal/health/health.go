package health

// ServiceStatus represents the health of a service.
type ServiceStatus string

const (
	StatusOK       ServiceStatus = "OK"
	StatusDegraded ServiceStatus = "DEGRADED"
	StatusFailed   ServiceStatus = "FAILED"
)

// DriftKind captures the type of drift detected.
type DriftKind string

const (
	DriftMissing      DriftKind = "MISSING"
	DriftExtra        DriftKind = "EXTRA"
	DriftExtraService DriftKind = "EXTRA_SERVICE"
)

// DriftDetail describes a single drift finding.
type DriftDetail struct {
	Kind     DriftKind
	Resource string
	Name     string
}

// ServiceHealth captures health evaluation output for a service.
type ServiceHealth struct {
	Name    string
	Status  ServiceStatus
	Reasons []string
	Drift   []DriftDetail
}

// StackHealth summarizes the health for a stack.
type StackHealth struct {
	Status   ServiceStatus
	Services map[string]ServiceHealth
}
