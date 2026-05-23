package config

import "testing"

func TestLoad_distillationPolicyDisabled(t *testing.T) {
	t.Setenv("DISTILLATION_POLICY_DISABLED", "true")
	cfg := Load()
	if !cfg.DistillationPolicyDisabled {
		t.Fatal("expected DistillationPolicyDisabled true")
	}
	t.Setenv("DISTILLATION_POLICY_DISABLED", "")
	cfg = Load()
	if cfg.DistillationPolicyDisabled {
		t.Fatal("expected default false")
	}
}
