package worker

import "testing"

func TestParentPrefixRoot(t *testing.T) {
	cases := []struct{ in, want string }{
		{"root", "root"},
		{"", "root"},
		{"   ", "root"},
		{"root.a", "root"},
		{"root.a.b", "root.a"},
		{"root.a.b.c.d", "root.a.b.c"},
		{"single", "root"}, // no dot → fallback
	}
	for _, tc := range cases {
		got := parentPrefix(tc.in)
		if got != tc.want {
			t.Errorf("parentPrefix(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTriggerForMemoryEmptyInputs(t *testing.T) {
	// With empty tenantID or path the method should return immediately (no DB call → no panic).
	w := &ConsolidationWorker{db: nil}
	w.TriggerForMemory("", "root.a")
	w.TriggerForMemory("tid", "")
	w.TriggerForMemory("", "")
}
