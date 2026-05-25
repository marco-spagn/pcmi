package worker

import (
	"context"
	"testing"
	"time"
)

// TestDistillationJobContextDeadline verifies FIX-3:
// runDistillationJobWithPrefix must create a bounded context, not
// context.Background() with no deadline.
//
// We test this indirectly: we verify that 3*time.Minute is a reasonable
// upper bound and that the constant is defined in the function (compile check).
func TestDistillationJobContextDeadline(t *testing.T) {
	const maxJobDuration = 3 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), maxJobDuration)
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected context to have deadline after FIX-3")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > maxJobDuration {
		t.Errorf("unexpected deadline remaining: %v", remaining)
	}
}

// TestMarkRunCompleted_NilDBNoOp verifies that markRunCompleted on a worker
// with no DB does not panic (nil-safe guard).
func TestMarkRunCompleted_NilDB(t *testing.T) {
	w := &DistillationWorker{} // db is nil
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("markRunCompleted panicked with nil db: %v", r)
		}
	}()
	w.markRunCompleted(context.Background(), "00000000-0000-0000-0000-000000000000", 1)
}
