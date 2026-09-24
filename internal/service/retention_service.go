package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/marco-spagn/pcmi/internal/model"
)

// RetentionStore persists retention policies and performs erasure.
type RetentionStore interface {
	List(ctx context.Context, tenantID string) ([]model.RetentionPolicy, error)
	Upsert(ctx context.Context, tenantID string, req model.RetentionPolicyRequest) (model.RetentionPolicy, error)
	Delete(ctx context.Context, tenantID, pathPrefix string) error
	Erase(ctx context.Context, tenantID string, req model.EraseRequest, apiKeyID string) (model.EraseCounts, []string, error)
	EraseGraphVertices(ctx context.Context, tenantID string, vertexIDs []string) (int, error)
}

// ErrInvalidRetentionRequest wraps request validation failures (HTTP 400).
var ErrInvalidRetentionRequest = errors.New("invalid request")

// RetentionService validates retention policy changes and orchestrates GDPR
// erasure (SQL erase in one transaction, then best-effort graph cleanup).
type RetentionService struct {
	store RetentionStore
	now   func() time.Time
}

func NewRetentionService(store RetentionStore) *RetentionService {
	return &RetentionService{store: store, now: time.Now}
}

func (s *RetentionService) List(ctx context.Context, tenantID string) ([]model.RetentionPolicy, error) {
	return s.store.List(ctx, tenantID)
}

func (s *RetentionService) Upsert(ctx context.Context, tenantID string, req model.RetentionPolicyRequest) (model.RetentionPolicy, error) {
	if err := req.Normalize(); err != nil {
		return model.RetentionPolicy{}, fmt.Errorf("%w: %v", ErrInvalidRetentionRequest, err)
	}
	return s.store.Upsert(ctx, tenantID, req)
}

func (s *RetentionService) Delete(ctx context.Context, tenantID, pathPrefix string) error {
	if err := model.ValidateRetentionPrefix(pathPrefix); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRetentionRequest, err)
	}
	return s.store.Delete(ctx, tenantID, pathPrefix)
}

// Erase removes every version of the memories under req.PathPrefix and their
// derived data. Graph cleanup failures do not undo the committed SQL erasure;
// they are reported in EraseResult.GraphError so an operator can retry.
func (s *RetentionService) Erase(ctx context.Context, tenantID string, req model.EraseRequest, apiKeyID string) (model.EraseResult, error) {
	if err := req.Normalize(); err != nil {
		return model.EraseResult{}, fmt.Errorf("%w: %v", ErrInvalidRetentionRequest, err)
	}
	counts, vertexIDs, err := s.store.Erase(ctx, tenantID, req, apiKeyID)
	if err != nil {
		return model.EraseResult{}, err
	}
	res := model.EraseResult{PathPrefix: req.PathPrefix, DryRun: req.DryRun, Counts: counts}
	if req.DryRun {
		return res, nil
	}
	at := s.now().UTC()
	res.ErasedAt = &at
	n, gerr := s.store.EraseGraphVertices(ctx, tenantID, vertexIDs)
	res.GraphVertices = n
	if gerr != nil {
		log.Printf("erase: graph cleanup for tenant %s prefix %s: %v", tenantID, req.PathPrefix, gerr)
		res.GraphError = gerr.Error()
	}
	return res, nil
}
