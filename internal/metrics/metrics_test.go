package metrics

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsUpdates(t *testing.T) {
	m := New()

	m.ObserveCycleDuration(2 * time.Second)
	m.SetServicesTotal("alpha", "ok", 3)
	m.SetServicesTotal("alpha", "failed", 1)
	m.IncAlertsTotal("alpha", "failed")
	m.IncDockerAPIErrors()
	m.SetLastSuccessfulCycleTimestamp(time.Unix(100, 0))

	if got := testutil.ToFloat64(m.servicesTotal.WithLabelValues("alpha", "ok")); got != 3 {
		t.Fatalf("expected ok services 3, got %v", got)
	}
	if got := testutil.ToFloat64(m.servicesTotal.WithLabelValues("alpha", "failed")); got != 1 {
		t.Fatalf("expected failed services 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.alertsTotal.WithLabelValues("alpha", "failed")); got != 1 {
		t.Fatalf("expected alerts 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.dockerAPIErrorsTotal); got != 1 {
		t.Fatalf("expected docker api errors 1, got %v", got)
	}
	if got := testutil.ToFloat64(m.lastSuccessfulCycleGauge); got != 100 {
		t.Fatalf("expected last successful cycle 100, got %v", got)
	}
	if count := testutil.CollectAndCount(m.cycleDurationSeconds); count == 0 {
		t.Fatalf("expected cycle duration histogram to be collected")
	}
}
