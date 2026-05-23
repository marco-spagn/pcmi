package repository

import (
	"fmt"
	"math"
	"strconv"
)

// temporalClause restricts rows to the current slice or a point-in-time view.
// asOfParam is a query placeholder such as "$3"; a timestamptz cast is applied for NULL inference.
func temporalClause(asOfParam string) string {
	asOf := asOfParam + "::timestamptz"
	return `(
		(` + asOf + ` IS NULL AND valid_to IS NULL)
		OR
		(` + asOf + ` IS NOT NULL AND valid_from <= ` + asOf + ` AND (valid_to IS NULL OR valid_to > ` + asOf + `))
	)`
}

// scopeFilters adds optional cross-agent and embedding-space predicates.
// Empty string parameters mean "no filter" (avoids untyped NULL uuid bind issues).
func scopeFilters(agentParam, spaceParam string) string {
	return `($` + agentParam + ` = '' OR source_agent_id = $` + agentParam + `::uuid)
		  AND ($` + spaceParam + ` = '' OR embedding_space = $` + spaceParam + `)`
}

// tagFilters restricts rows by tag overlap (any) or containment (all).
// tagsParam is a text[] placeholder; matchParam is '' | 'any' | 'all'.
func tagFilters(tagsParam, matchParam string) string {
	return `(
		cardinality($` + tagsParam + `::text[]) = 0
		OR (
			($` + matchParam + ` IN ('', 'any') AND tags && $` + tagsParam + `::text[])
			OR ($` + matchParam + ` = 'all' AND tags @> $` + tagsParam + `::text[])
		)
	)`
}

// memoryEntrySelectCols is the standard column list for scanMemoryEntry (without relevance_score).
const memoryEntrySelectCols = `id, tenant_id, path, content, metadata, tags, embedding, embedding_model, embedding_space,
	       version, valid_from, valid_to, source_agent_id, source_event_id::text, created_at, content_encrypted,
	       importance, access_count, last_accessed_at`

const (
	defaultWeightSemantic    = 0.40
	defaultWeightLexical     = 0.30
	defaultWeightImportance  = 0.15
	defaultWeightRecency     = 0.15
	defaultDecayHalflifeDays = 30.0
)

// ScoringWeights configures hybrid retrieval fusion (semantic + lexical + importance + recency).
type ScoringWeights struct {
	Semantic, Lexical, Importance, Recency float64
	HalflifeDays                           float64
	DecayEnabled                           bool
}

// DefaultScoringWeights returns tenant defaults when no row exists in tenant_memory_config.
func DefaultScoringWeights(decayEnabled bool) ScoringWeights {
	return ScoringWeights{
		Semantic:     defaultWeightSemantic,
		Lexical:      defaultWeightLexical,
		Importance:   defaultWeightImportance,
		Recency:      defaultWeightRecency,
		HalflifeDays: defaultDecayHalflifeDays,
		DecayEnabled: decayEnabled,
	}
}

// LegacyHybridWeights preserves pre-v1.42 semantic/lexical-only fusion (no importance/recency).
func LegacyHybridWeights() ScoringWeights {
	return ScoringWeights{Semantic: 0.55, Lexical: 0.45, DecayEnabled: false}
}

// RecencyFactor computes exp(-ln(2)/halflife * ageDays) for temporal decay.
func RecencyFactor(ageDays, halflifeDays float64) float64 {
	if halflifeDays <= 0 {
		return 1
	}
	if ageDays < 0 {
		ageDays = 0
	}
	return math.Exp(-math.Ln2 / halflifeDays * ageDays)
}

// HybridScore fuses ranking signals in Go (mirrors hybridScoreSQL for unit tests).
func HybridScore(w ScoringWeights, cosine, bm25, importance, ageDays float64) float64 {
	rec := 0.0
	if w.DecayEnabled && w.Recency > 0 {
		rec = w.Recency * RecencyFactor(ageDays, w.HalflifeDays)
	}
	return w.Semantic*cosine + w.Lexical*bm25 + w.Importance*importance + rec
}

func ageDaysSQL() string {
	return `GREATEST(EXTRACT(EPOCH FROM (NOW() - COALESCE(last_accessed_at, created_at))) / 86400.0, 0)::float8`
}

func formatWeight(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// hybridScoreSQL builds the fused relevance_score expression used in retrieve queries.
func hybridScoreSQL(w ScoringWeights, cosineExpr, bm25Expr string) string {
	recExpr := "0"
	if w.DecayEnabled && w.Recency > 0 && w.HalflifeDays > 0 {
		recExpr = fmt.Sprintf("(%s * EXP(-LN(2) / %s * %s))",
			formatWeight(w.Recency), formatWeight(w.HalflifeDays), ageDaysSQL())
	}
	return fmt.Sprintf(`(
		%s * (%s)
		+ %s * (%s)
		+ %s * COALESCE(importance, 0.5)
		+ %s
	)::float8`,
		formatWeight(w.Semantic), cosineExpr,
		formatWeight(w.Lexical), bm25Expr,
		formatWeight(w.Importance),
		recExpr,
	)
}
