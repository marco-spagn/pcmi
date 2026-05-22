package service

import (
	"context"
	"testing"

	"github.com/marco-spagn/pcmi/internal/event"
	"github.com/marco-spagn/pcmi/internal/model"
)

func TestMemoryService_Store_publishFailsStillReturnsResult(t *testing.T) {
	event.RedisClient = nil
	repo := &fullMockRepo{
		storeFn: func(model.StoreRequest) (int64, int, *int64, error) {
			return 42, 1, nil, nil
		},
	}
	svc := NewMemoryService(repo, nil)
	res, err := svc.Store(context.Background(), &model.StoreRequest{Path: "root.test", Content: "hi"}, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || res.Entry.ID != 42 {
		t.Fatalf("unexpected result %+v", res)
	}
}
