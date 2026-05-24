package worker

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDistillationStress_SemaphoreUnder1000Jobs fires 1000 goroutines against
// a semaphore of capacity 8. Peak concurrency must never exceed 8.
func TestDistillationStress_SemaphoreUnder1000Jobs(t *testing.T) {
	t.Parallel()
	const (
		maxConcurrent = 8
		numJobs       = 1000
	)
	sem := make(chan struct{}, maxConcurrent)
	var active atomic.Int32
	var peak atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fakeJob(sem, time.Millisecond, &active, &peak)
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("TestDistillationStress_SemaphoreUnder1000Jobs: deadlock after 30s")
	}

	if p := peak.Load(); p > int32(maxConcurrent) {
		t.Errorf("peak concurrency %d exceeded semaphore capacity %d", p, maxConcurrent)
	}
	if active.Load() != 0 {
		t.Errorf("active counter not zeroed after all jobs: %d", active.Load())
	}
}

// TestDistillationStress_SemaphorePeakMonotonic tests that at any point
// during execution the concurrency is in [0, maxConcurrent].
// A goroutine samples the active counter 500 times while jobs run.
func TestDistillationStress_SemaphorePeakMonotonic(t *testing.T) {
	t.Parallel()
	const (
		maxConcurrent = 4
		numJobs       = 500
	)
	sem := make(chan struct{}, maxConcurrent)
	var active atomic.Int32
	var peak atomic.Int32
	var violations atomic.Int32

	// Observer goroutine.
	stopObs := make(chan struct{})
	go func() {
		for {
			select {
			case <-stopObs:
				return
			default:
				cur := active.Load()
				if cur < 0 || cur > int32(maxConcurrent) {
					violations.Add(1)
				}
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fakeJob(sem, 2*time.Millisecond, &active, &peak)
		}()
	}
	wg.Wait()
	close(stopObs)

	if v := violations.Load(); v > 0 {
		t.Errorf("observed %d concurrency violations (active out of [0,%d])", v, maxConcurrent)
	}
}

// TestDistillationStress_DistillPathPrefixConcurrent calls DistillPathPrefix
// from 100 goroutines × 100 calls each with varied inputs. Must produce
// no panics and always return a non-empty string.
func TestDistillationStress_DistillPathPrefixConcurrent(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"",
		"root",
		"root.test",
		"root.test.foo",
		"root.test.foo.bar",
		"a.b.c.d.e.f",
		"single",
		"  whitespace  ",
		"root.test.deeply.nested.path",
	}

	var wg sync.WaitGroup
	var emptyResults atomic.Int32

	for g := 0; g < 100; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				inp := inputs[(g*100+i)%len(inputs)]
				result := DistillPathPrefix(inp)
				if result == "" {
					emptyResults.Add(1)
				}
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestDistillationStress_DistillPathPrefixConcurrent: deadlock")
	}

	if n := emptyResults.Load(); n > 0 {
		t.Errorf("DistillPathPrefix returned empty string %d times", n)
	}
}

// TestDistillationStress_ParseLLMResponseConcurrent fires 200 goroutines
// parsing a mix of valid JSON, trailing-comma JSON, and garbage strings.
// Verifies no data races and that valid inputs always succeed.
func TestDistillationStress_ParseLLMResponseConcurrent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input   string
		wantErr bool
	}{
		{`{"summary":"ok","insights":["a","b"]}`, false},
		{`{"summary":"ok","insights":["a","b"],}`, false}, // repaired
		{"```json\n{\"summary\":\"s\",\"insights\":[]}\n```", false},
		{`{"summary":"no insights"}`, false},
		{`not json at all`, true},
		{`{}`, false},
		{`{"summary":"","insights":[]}`, false},
		{`{"summary":"s","insights":["x"` /* truncated */, true},
	}

	var wg sync.WaitGroup
	var unexpectedErrors atomic.Int32
	var unexpectedOK atomic.Int32

	for g := 0; g < 200; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i, tc := range cases {
				_, err := parseDistillationLLMResponse(tc.input)
				if tc.wantErr && err == nil {
					unexpectedOK.Add(1)
					t.Logf("goroutine %d case %d: expected error, got none for input %q", g, i, tc.input)
				}
				if !tc.wantErr && err != nil {
					unexpectedErrors.Add(1)
					t.Logf("goroutine %d case %d: unexpected error: %v (input %q)", g, i, err, tc.input)
				}
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestDistillationStress_ParseLLMResponseConcurrent: deadlock")
	}

	if n := unexpectedErrors.Load(); n > 0 {
		t.Errorf("%d unexpected parse errors on valid input", n)
	}
}

// TestDistillationStress_BatchAndConcurrencyValidation exercises
// distillationBatchSizeFrom and distillationConcurrencyFrom with every
// valid and invalid boundary value from 500 goroutines. The output must
// always be in the valid range.
func TestDistillationStress_BatchAndConcurrencyValidation(t *testing.T) {
	t.Parallel()
	batchInputs := []int{-999, -1, 0, 1, 5, 10, 100, 200, 201, 999}
	concInputs := []int{-99, 0, 1, 4, 8, 16, 17, 99}

	var wg sync.WaitGroup
	var batchViolations atomic.Int32
	var concViolations atomic.Int32

	for g := 0; g < 500; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i, v := range batchInputs {
				got := distillationBatchSizeFrom(v)
				if got < 1 || got > 200 {
					batchViolations.Add(1)
					t.Logf("g=%d i=%d batchSizeFrom(%d)=%d out of [1,200]", g, i, v, got)
				}
			}
			for i, v := range concInputs {
				got := distillationConcurrencyFrom(v)
				if got < 1 || got > 16 {
					concViolations.Add(1)
					t.Logf("g=%d i=%d concurrencyFrom(%d)=%d out of [1,16]", g, i, v, got)
				}
			}
		}(g)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestDistillationStress_BatchAndConcurrencyValidation: deadlock")
	}

	if n := batchViolations.Load(); n > 0 {
		t.Errorf("%d batch size values outside [1,200]", n)
	}
	if n := concViolations.Load(); n > 0 {
		t.Errorf("%d concurrency values outside [1,16]", n)
	}
}

// TestDistillationStress_SemaphoreNoDeadlockOnRelease verifies that the
// acquire→work→release cycle is always symmetric even under high contention.
// We deliberately vary job durations to maximise scheduling interleaving.
func TestDistillationStress_SemaphoreNoDeadlockOnRelease(t *testing.T) {
	t.Parallel()
	const (
		maxConcurrent = 6
		numJobs       = 800
	)
	sem := make(chan struct{}, maxConcurrent)
	var active atomic.Int32
	var peak atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < numJobs; i++ {
		wg.Add(1)
		delay := time.Duration(i%5) * time.Millisecond
		go func(d time.Duration) {
			defer wg.Done()
			fakeJob(sem, d, &active, &peak)
		}(delay)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("TestDistillationStress_SemaphoreNoDeadlockOnRelease: deadlock (peak=%d)", peak.Load())
	}

	// After all goroutines exit, semaphore must be fully available.
	for i := 0; i < maxConcurrent; i++ {
		select {
		case sem <- struct{}{}:
		default:
			t.Fatalf("semaphore slot %d unavailable after all jobs completed — capacity leaked", i)
		}
	}
}

// TestDistillationStress_DistillPathPrefixEdgeCases tests specific edge-case
// inputs that could cause panics (empty strings, single segments, very long paths).
func TestDistillationStress_DistillPathPrefixEdgeCases(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"", "root.test"},
		{"root.test", "root.test"},
		{"root.test.foo", "root.test"},
		{"root.test.anything.deep", "root.test"},
		{"a.b", "a.b"},
		{"a.b.c", "a.b"},
		{"single", "single"},
		{"  ", "root.test"}, // whitespace only → empty after trim → default
	}

	var wg sync.WaitGroup
	for _, tc := range cases {
		for i := 0; i < 200; i++ {
			wg.Add(1)
			tc := tc
			go func() {
				defer wg.Done()
				got := DistillPathPrefix(tc.input)
				if got != tc.want {
					t.Errorf("DistillPathPrefix(%q) = %q, want %q", tc.input, got, tc.want)
				}
			}()
		}
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestDistillationStress_DistillPathPrefixEdgeCases: deadlock")
	}
}

// TestDistillationStress_WorkerConcurrencyConfigVariants creates workers with
// every valid concurrency value and verifies semaphore capacity matches.
func TestDistillationStress_WorkerConcurrencyConfigVariants(t *testing.T) {
	t.Parallel()
	var wg sync.WaitGroup
	for c := 1; c <= 16; c++ {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := newTestWorker(c)
			if cap(w.sem) != c {
				t.Errorf("newTestWorker(%d): sem capacity=%d, want %d", c, cap(w.sem), c)
			}
			// Verify all slots are usable.
			for i := 0; i < c; i++ {
				w.sem <- struct{}{}
			}
			for i := 0; i < c; i++ {
				<-w.sem
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("TestDistillationStress_WorkerConcurrencyConfigVariants: deadlock")
	}
}

// TestDistillationStress_LargeJSONRepair calls repairDistillationJSON with
// 300 goroutines on inputs with increasingly complex trailing-comma patterns.
func TestDistillationStress_LargeJSONRepair(t *testing.T) {
	t.Parallel()
	// Build a JSON with 50 insights and a trailing comma.
	var parts []string
	for i := 0; i < 50; i++ {
		parts = append(parts, fmt.Sprintf(`"insight number %d"`, i))
	}
	malformed := `{"summary":"big","insights":[` + joinStrings(parts, ",") + `,]}`

	var wg sync.WaitGroup
	var parseErrors atomic.Int32
	for g := 0; g < 300; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := parseDistillationLLMResponse(malformed)
			if err != nil {
				parseErrors.Add(1)
			}
		}()
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("TestDistillationStress_LargeJSONRepair: deadlock")
	}

	if n := parseErrors.Load(); n > 0 {
		t.Errorf("%d parse errors on repairable JSON", n)
	}
}

func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}
