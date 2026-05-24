package worker

import "testing"

func TestDistillPathPrefix(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "root.test"},
		{"root.test.foo", "root.test"},
		{"root.ci.smoke", "root.ci"},
		{"solo", "solo"},
	}
	for _, tc := range tests {
		if got := DistillPathPrefix(tc.in); got != tc.want {
			t.Errorf("DistillPathPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSourceIDsEqual(t *testing.T) {
	if !sourceIDsEqual([]int64{3, 1, 2}, []int64{2, 3, 1}) {
		t.Fatal("expected equal source id sets")
	}
	if sourceIDsEqual([]int64{1}, []int64{1, 2}) {
		t.Fatal("expected different lengths to be unequal")
	}
}

func TestParseDistillationLLMResponse(t *testing.T) {
	t.Parallel()
	valid := `{"summary":"ok","insights":["a","b"]}`
	got, err := parseDistillationLLMResponse(valid)
	if err != nil || got.Summary != "ok" || len(got.Insights) != 2 {
		t.Fatalf("valid JSON: err=%v got=%+v", err, got)
	}

	// Malformed payload from ci-e2e logs (missing closing ] on insights array).
	malformed := `{"summary": "Knowledge distillation event triggered, identified by unique event ID 1779627053 within an SSE E2E framework.", "insights": ["Knowledge distillation can enhance model performance by transferring knowledge from a larger model to a smaller one.", "Event-driven architectures can facilitate real-time updates and processing of knowledge."}`
	got, err = parseDistillationLLMResponse(malformed)
	if err != nil {
		t.Fatalf("repair malformed JSON: %v", err)
	}
	if got.Summary == "" || len(got.Insights) != 2 {
		t.Fatalf("unexpected parse result: %+v", got)
	}

	trailingComma := `{"summary":"s","insights":["one","two",]}`
	got, err = parseDistillationLLMResponse(trailingComma)
	if err != nil || len(got.Insights) != 2 {
		t.Fatalf("trailing comma repair: err=%v got=%+v", err, got)
	}
}
