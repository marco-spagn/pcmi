package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/marco-spagn/pcmi/internal/model"
)

type retrieveErrRepo struct{ fullMockRepo }

func (r *retrieveErrRepo) Retrieve(context.Context, model.RetrieveRequest, string, []float32) ([]model.MemoryEntry, error) {
	return nil, errors.New("db retrieve failed")
}

func TestMemoryService_Retrieve_RepoError(t *testing.T) {
	svc := NewMemoryService(&retrieveErrRepo{}, nil)
	_, err := svc.Retrieve(context.Background(), &model.RetrieveRequest{PathPrefix: "root"}, "tid")
	if err == nil || !strings.Contains(err.Error(), "retrieve failed") {
		t.Fatalf("err=%v", err)
	}
}

func TestMemoryService_GetByPath_NotFound(t *testing.T) {
	repo := &fullMockRepo{
		getByPathFn: func(string) (*model.MemoryEntry, error) {
			return nil, errors.New("memory not found")
		},
	}
	svc := NewMemoryService(repo, nil)
	_, err := svc.GetByPath(context.Background(), "tid", "missing", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err=%v", err)
	}
}
