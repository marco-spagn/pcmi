package metrics

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
)

// TestIncWorkerRedisEventUnknownLabel exercises the empty-string branch of
// IncWorkerRedisEvent, which relabels "" → "unknown" so an event missing its
// type doesn't pollute the metric namespace with an empty label.
func TestIncWorkerRedisEventUnknownLabel(t *testing.T) {
	IncWorkerRedisEvent("")

	mfs, err := WorkerRegistry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var counterFamily *dto.MetricFamily
	for _, mf := range mfs {
		if mf.GetName() == "pcmi_worker_redis_events_total" {
			counterFamily = mf
			break
		}
	}
	if counterFamily == nil {
		t.Fatal("pcmi_worker_redis_events_total not registered")
	}
	foundUnknown := false
	for _, m := range counterFamily.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "event_type" && lp.GetValue() == "unknown" {
				foundUnknown = true
			}
		}
	}
	if !foundUnknown {
		t.Fatal(`empty event_type should be relabelled to "unknown"`)
	}
}

func TestIncWorkerRedisEventCustomLabel(t *testing.T) {
	IncWorkerRedisEvent("custom.event.type")

	mfs, err := WorkerRegistry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "pcmi_worker_redis_events_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "event_type" && lp.GetValue() == "custom.event.type" {
					return
				}
			}
		}
	}
	t.Fatal(`custom event_type "custom.event.type" was not recorded`)
}
