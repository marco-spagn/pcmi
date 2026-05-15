package service

import (
	"context"
	"fmt"

	"github.com/marco-spagn/pcmi/internal/metrics"
	"github.com/marco-spagn/pcmi/internal/model"
)

func (s *MemoryService) BatchStore(ctx context.Context, req *model.BatchStoreRequest, tenantID string) (*model.BatchStoreResult, error) {
	if len(req.Items) == 0 {
		return nil, fmt.Errorf("items required")
	}
	if len(req.Items) > 50 {
		return nil, fmt.Errorf("maximum 50 items per batch")
	}
	out := &model.BatchStoreResult{Total: len(req.Items)}
	for i, item := range req.Items {
		res, err := s.Store(ctx, &item, tenantID)
		if err != nil {
			out.Results = append(out.Results, model.BatchStoreItemResult{
				Index: i, Status: "error", Error: err.Error(),
			})
			continue
		}
		metrics.IncStore()
		out.Results = append(out.Results, model.BatchStoreItemResult{
			Index: i, ID: res.Entry.ID, Status: "stored",
			Version: res.Version, SupersededID: res.SupersededID,
		})
	}
	return out, nil
}

func (s *MemoryService) BatchRetrieve(ctx context.Context, req *model.BatchRetrieveRequest, tenantID string) (*model.BatchRetrieveResponse, error) {
	if len(req.Queries) == 0 {
		return nil, fmt.Errorf("queries required")
	}
	if len(req.Queries) > 20 {
		return nil, fmt.Errorf("maximum 20 queries per batch")
	}
	out := &model.BatchRetrieveResponse{Total: len(req.Queries)}
	for _, q := range req.Queries {
		r, err := s.Retrieve(ctx, &q, tenantID)
		if err != nil {
			return nil, err
		}
		metrics.IncRetrieve()
		out.Results = append(out.Results, *r)
	}
	return out, nil
}
