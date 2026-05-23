package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// GetScoringWeights loads per-tenant fusion weights; defaults apply when no config row exists.
func (r *MemoryRepository) GetScoringWeights(ctx context.Context, tenantID string, decayEnabled bool) (ScoringWeights, error) {
	w := DefaultScoringWeights(decayEnabled)
	row := r.r.QueryRow(ctx, `
		SELECT weight_semantic, weight_lexical, weight_importance, weight_recency, decay_halflife_days
		FROM tenant_memory_config
		WHERE tenant_id = $1::uuid`, tenantID)
	err := row.Scan(&w.Semantic, &w.Lexical, &w.Importance, &w.Recency, &w.HalflifeDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, nil
	}
	if err != nil {
		return w, fmt.Errorf("load scoring weights: %w", err)
	}
	w.DecayEnabled = decayEnabled
	return w, nil
}
