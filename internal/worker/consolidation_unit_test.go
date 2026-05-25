package worker

import (
	"testing"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

var testNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func TestNormalizeSourceIDs_Dedup(t *testing.T) {
	in := []int64{5, 1, 3, 1, 5}
	out := normalizeSourceIDs(in)
	// normalizeSourceIDs sorts but does NOT dedup — verify sort order.
	if len(out) != len(in) {
		t.Fatalf("len=%d, want %d", len(out), len(in))
	}
	for i := 1; i < len(out); i++ {
		if out[i] < out[i-1] {
			t.Errorf("not sorted at index %d: %v", i, out)
		}
	}
	// original must be untouched
	if in[0] != 5 {
		t.Error("normalizeSourceIDs must not mutate its argument")
	}
}

func TestSourceIDsEqual_Nil(t *testing.T) {
	if !sourceIDsEqual(nil, nil) {
		t.Error("nil == nil should be true")
	}
	if sourceIDsEqual(nil, []int64{1}) {
		t.Error("nil != [1]")
	}
	if sourceIDsEqual([]int64{1}, nil) {
		t.Error("[1] != nil")
	}
}

func TestShouldTriggerPolicy_AllBranches(t *testing.T) {
	makePolicy := func(enabled bool, count, threshold int) model.DistillationPolicy {
		return model.DistillationPolicy{
			Enabled:        enabled,
			CountThreshold: threshold,
		}
	}

	cases := []struct {
		name        string
		policy      model.DistillationPolicy
		state       PolicyEvalState
		wantTrigger bool
		wantReason  string
	}{
		{
			"disabled",
			makePolicy(false, 10, 5),
			PolicyEvalState{MemoryCount: 10},
			false, "disabled",
		},
		{
			"count met",
			makePolicy(true, 10, 5),
			PolicyEvalState{MemoryCount: 10},
			true, "count_threshold",
		},
		{
			"under threshold",
			makePolicy(true, 3, 5),
			PolicyEvalState{MemoryCount: 3},
			false, "under_threshold",
		},
		{
			"min interval not elapsed",
			model.DistillationPolicy{
				Enabled:         true,
				CountThreshold:  1,
				MinIntervalSecs: 300,
				LastTriggeredAt: func() *time.Time { t := testNow.Add(-10 * time.Second); return &t }(),
			},
			PolicyEvalState{MemoryCount: 10},
			false, "min_interval",
		},
		{
			"age trigger",
			model.DistillationPolicy{
				Enabled:        true,
				CountThreshold: 100,
				MaxAgeSecs:     func() *int { v := 60; return &v }(),
			},
			PolicyEvalState{MemoryCount: 0, OldestEntryAge: 2 * time.Minute},
			true, "max_age",
		},
	}

	for _, tc := range cases {
		ok, reason := ShouldTriggerPolicy(tc.policy, tc.state, testNow)
		if ok != tc.wantTrigger || reason != tc.wantReason {
			t.Errorf("%s: got (%v,%q) want (%v,%q)", tc.name, ok, reason, tc.wantTrigger, tc.wantReason)
		}
	}
}
