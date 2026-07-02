package worker_test

import (
	"testing"

	"github.com/marco-spagn/pcmi/internal/config"
	"github.com/marco-spagn/pcmi/internal/worker"
)

func TestExtractionWorker_disabledByDefault(t *testing.T) {
	w := worker.NewExtractionWorker(nil, &config.Config{ExtractionEnabled: false})
	if w.Enabled() {
		t.Fatal("expected disabled worker")
	}
}

func TestExtractionWorker_enabled(t *testing.T) {
	w := worker.NewExtractionWorker(nil, &config.Config{ExtractionEnabled: true})
	if !w.Enabled() {
		t.Fatal("expected enabled worker")
	}
}
