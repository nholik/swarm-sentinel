package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics wraps Prometheus collectors for swarm-sentinel.
type Metrics struct {
	registry                 *prometheus.Registry
	cycleDurationSeconds     prometheus.Histogram
	servicesTotal            *prometheus.GaugeVec
	alertsTotal              *prometheus.CounterVec
	dockerAPIErrorsTotal     prometheus.Counter
	lastSuccessfulCycleGauge prometheus.Gauge
}

// New initializes a Metrics registry with all collectors registered.
func New() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		registry: registry,
		cycleDurationSeconds: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "swarm_sentinel_cycle_duration_seconds",
			Help:    "Duration of health evaluation cycles in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		servicesTotal: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "swarm_sentinel_services_total",
			Help: "Total services by stack and status.",
		}, []string{"stack", "status"}),
		alertsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "swarm_sentinel_alerts_total",
			Help: "Total alerts emitted by stack and severity.",
		}, []string{"stack", "severity"}),
		dockerAPIErrorsTotal: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "swarm_sentinel_docker_api_errors_total",
			Help: "Total Docker API errors after retries.",
		}),
		lastSuccessfulCycleGauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "swarm_sentinel_last_successful_cycle_timestamp",
			Help: "Unix timestamp of the last successful cycle.",
		}),
	}

	registry.MustRegister(
		m.cycleDurationSeconds,
		m.servicesTotal,
		m.alertsTotal,
		m.dockerAPIErrorsTotal,
		m.lastSuccessfulCycleGauge,
	)

	return m
}

// Handler returns a Prometheus HTTP handler for this registry.
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return promhttp.Handler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObserveCycleDuration records the duration of a completed cycle.
func (m *Metrics) ObserveCycleDuration(duration time.Duration) {
	if m == nil {
		return
	}
	m.cycleDurationSeconds.Observe(duration.Seconds())
}

// SetServicesTotal sets the services gauge for the given stack/status.
func (m *Metrics) SetServicesTotal(stack string, status string, value int) {
	if m == nil {
		return
	}
	m.servicesTotal.WithLabelValues(stack, status).Set(float64(value))
}

// IncAlertsTotal increments the alerts counter for the given stack/severity.
func (m *Metrics) IncAlertsTotal(stack string, severity string) {
	if m == nil {
		return
	}
	m.alertsTotal.WithLabelValues(stack, severity).Inc()
}

// IncDockerAPIErrors increments the Docker API error counter.
func (m *Metrics) IncDockerAPIErrors() {
	if m == nil {
		return
	}
	m.dockerAPIErrorsTotal.Inc()
}

// SetLastSuccessfulCycleTimestamp sets the last successful cycle time.
func (m *Metrics) SetLastSuccessfulCycleTimestamp(t time.Time) {
	if m == nil {
		return
	}
	m.lastSuccessfulCycleGauge.Set(float64(t.Unix()))
}
