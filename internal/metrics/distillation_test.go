package metrics

import (
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
)

// gather collects all metric families from WorkerRegistry.
func gatherWorker(t *testing.T) map[string]*dto.MetricFamily {
	t.Helper()
	mfs, err := WorkerRegistry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := make(map[string]*dto.MetricFamily, len(mfs))
	for _, mf := range mfs {
		out[mf.GetName()] = mf
	}
	return out
}

func TestObserveDistillationJobIncrementsCounter(t *testing.T) {
	ObserveDistillationJob(0.1, "ok")
	ObserveDistillationJob(0.2, "error")
	ObserveDistillationJob(0.05, "ok")

	mfs := gatherWorker(t)
	mf, ok := mfs["pcmi_distillation_total"]
	if !ok {
		t.Fatal("pcmi_distillation_total metric not found")
	}

	counts := map[string]float64{}
	for _, m := range mf.GetMetric() {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "status" {
				counts[lp.GetValue()] = m.GetCounter().GetValue()
			}
		}
	}
	if counts["ok"] < 2 {
		t.Errorf("expected ok count >= 2, got %v", counts["ok"])
	}
	if counts["error"] < 1 {
		t.Errorf("expected error count >= 1, got %v", counts["error"])
	}
}

func TestObserveDistillationDurationHistogram(t *testing.T) {
	ObserveDistillationJob(1.5, "ok")

	mfs := gatherWorker(t)
	if _, ok := mfs["pcmi_distillation_duration_seconds"]; !ok {
		t.Fatal("pcmi_distillation_duration_seconds metric not found")
	}
}

func TestObserveDistillationSourcesHistogram(t *testing.T) {
	ObserveDistillationSources(42)

	mfs := gatherWorker(t)
	mf, ok := mfs["pcmi_distillation_sources_per_job"]
	if !ok {
		t.Fatal("pcmi_distillation_sources_per_job not found")
	}
	h := mf.GetMetric()[0].GetHistogram()
	if h.GetSampleCount() == 0 {
		t.Error("expected at least 1 sample in sources histogram")
	}
}

func TestDistillationActiveGauge(t *testing.T) {
	IncDistillationActive()
	IncDistillationActive()
	DecDistillationActive()

	mfs := gatherWorker(t)
	mf, ok := mfs["pcmi_distillation_active_jobs"]
	if !ok {
		t.Fatal("pcmi_distillation_active_jobs not found")
	}
	val := mf.GetMetric()[0].GetGauge().GetValue()
	if val < 1 {
		t.Errorf("expected active jobs >= 1 after 2 inc + 1 dec, got %v", val)
	}
}

func TestDistillationQueuedGauge(t *testing.T) {
	IncDistillationQueued()
	DecDistillationQueued()

	mfs := gatherWorker(t)
	if _, ok := mfs["pcmi_distillation_queued_jobs"]; !ok {
		t.Fatal("pcmi_distillation_queued_jobs not found")
	}
}

func TestAllDistillationMetricsRegistered(t *testing.T) {
	mfs := gatherWorker(t)
	required := []string{
		"pcmi_distillation_duration_seconds",
		"pcmi_distillation_total",
		"pcmi_distillation_sources_per_job",
		"pcmi_distillation_active_jobs",
		"pcmi_distillation_queued_jobs",
	}
	for _, name := range required {
		if _, ok := mfs[name]; !ok {
			t.Errorf("metric %q not registered in WorkerRegistry", name)
		}
	}
}

func TestWorkerRegistryMetricNames(t *testing.T) {
	mfs := gatherWorker(t)
	for name := range mfs {
		if !strings.HasPrefix(name, "pcmi_") && !strings.HasPrefix(name, "go_") && !strings.HasPrefix(name, "process_") {
			t.Errorf("unexpected metric name without pcmi_ prefix: %s", name)
		}
	}
}
