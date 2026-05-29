package worker

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/marco-spagn/pcmi/internal/model"
)

// PolicyEvalState holds metrics used to decide whether a policy should fire.
type PolicyEvalState struct {
	MemoryCount     int
	OldestEntryAge  time.Duration
}

// ShouldTriggerPolicy returns whether distillation should run and a short reason code.
func ShouldTriggerPolicy(p model.DistillationPolicy, st PolicyEvalState, now time.Time) (bool, string) {
	if !p.Enabled {
		return false, "disabled"
	}
	if p.LastTriggeredAt != nil && p.MinIntervalSecs > 0 {
		elapsed := now.Sub(*p.LastTriggeredAt)
		if elapsed < time.Duration(p.MinIntervalSecs)*time.Second {
			return false, "min_interval"
		}
	}
	countOK := st.MemoryCount >= p.CountThreshold
	ageOK := false
	if p.MaxAgeSecs != nil && *p.MaxAgeSecs > 0 && st.OldestEntryAge > 0 {
		ageOK = st.OldestEntryAge >= time.Duration(*p.MaxAgeSecs)*time.Second
	}
	if countOK {
		return true, "count_threshold"
	}
	if ageOK {
		return true, "max_age"
	}
	return false, "under_threshold"
}

// DistillationPolicyEngine evaluates policies and triggers distillation jobs.
type DistillationPolicyEngine struct {
	db     workerDB
	dist   *DistillationWorker
	ticker time.Duration
}

func NewDistillationPolicyEngine(db workerDB, dist *DistillationWorker) *DistillationPolicyEngine {
	return &DistillationPolicyEngine{db: db, dist: dist, ticker: 30 * time.Second}
}

func (e *DistillationPolicyEngine) Start(ctx context.Context) {
	if e == nil || e.db == nil {
		return
	}
	ticker := time.NewTicker(e.ticker)
	defer ticker.Stop()
	log.Println("Distillation policy engine started (periodic age/count evaluation)")
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.evaluateAll(ctx)
		}
	}
}

// OnMemoryEvent evaluates policies matching the memory path after a store/update.
func (e *DistillationPolicyEngine) OnMemoryEvent(tenantID, memoryPath string) {
	if e == nil || e.db == nil || tenantID == "" {
		return
	}
	prefix := DistillPathPrefix(memoryPath)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := e.evaluatePrefix(ctx, tenantID, prefix); err != nil {
			log.Printf("distillation policy evaluate: %v", err)
		}
	}()
}

func (e *DistillationPolicyEngine) evaluateAll(ctx context.Context) {
	rows, err := e.db.Query(ctx, `
		SELECT id, tenant_id::text, name, path_prefix::text, enabled,
		       count_threshold, min_interval_secs, max_age_secs, last_triggered_at,
		       created_at, updated_at
		FROM distillation_policies
		WHERE enabled = true
		ORDER BY id ASC
		LIMIT 200`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanDistillationPolicy(rows)
		if err != nil {
			continue
		}
		_, _ = e.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", p.TenantID)
		st, err := e.loadEvalState(ctx, p.TenantID, p.PathPrefix)
		if err != nil {
			continue
		}
		e.maybeTrigger(ctx, p, st)
	}
}

func (e *DistillationPolicyEngine) evaluatePrefix(ctx context.Context, tenantID, pathPrefix string) error {
	if _, err := e.db.Exec(ctx, "SELECT set_tenant_context($1::uuid)", tenantID); err != nil {
		return err
	}
	policies, err := e.listPoliciesForPrefix(ctx, tenantID, pathPrefix)
	if err != nil {
		return err
	}
	if len(policies) == 0 {
		if e.dist != nil {
			e.dist.TriggerForPrefix(tenantID, pathPrefix)
		}
		return nil
	}
	st, err := e.loadEvalState(ctx, tenantID, pathPrefix)
	if err != nil {
		return err
	}
	for _, p := range policies {
		e.maybeTrigger(ctx, p, st)
	}
	return nil
}

func (e *DistillationPolicyEngine) listPoliciesForPrefix(ctx context.Context, tenantID, pathPrefix string) ([]model.DistillationPolicy, error) {
	rows, err := e.db.Query(ctx, `
		SELECT id, tenant_id::text, name, path_prefix::text, enabled,
		       count_threshold, min_interval_secs, max_age_secs, last_triggered_at,
		       created_at, updated_at
		FROM distillation_policies
		WHERE tenant_id = $1::uuid
		  AND enabled = true
		  AND $2::ltree <@ path_prefix
		ORDER BY nlevel(path_prefix) DESC`, tenantID, pathPrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DistillationPolicy
	for rows.Next() {
		p, err := scanDistillationPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (e *DistillationPolicyEngine) loadEvalState(ctx context.Context, tenantID, pathPrefix string) (PolicyEvalState, error) {
	var st PolicyEvalState
	var oldestSecs float64
	err := e.db.QueryRow(ctx, `
		SELECT COUNT(*)::int,
		       COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)::float8
		FROM memory_entries
		WHERE tenant_id = $1::uuid
		  AND path <@ $2::ltree
		  AND valid_to IS NULL`, tenantID, pathPrefix).Scan(&st.MemoryCount, &oldestSecs)
	if err != nil {
		return st, err
	}
	st.OldestEntryAge = time.Duration(oldestSecs * float64(time.Second))
	return st, nil
}

func (e *DistillationPolicyEngine) maybeTrigger(ctx context.Context, p model.DistillationPolicy, st PolicyEvalState) {
	ok, reason := ShouldTriggerPolicy(p, st, time.Now())
	if !ok {
		return
	}
	runID, err := e.beginRun(ctx, p, st.MemoryCount)
	if err != nil {
		log.Printf("distillation policy run begin: %v", err)
		return
	}
	if err := e.markPolicyTriggered(ctx, p.ID); err != nil {
		log.Printf("distillation policy last_triggered_at: %v", err)
	}
	if e.dist != nil {
		e.dist.TriggerForPrefix(p.TenantID, p.PathPrefix)
	}
	if err := e.completeRun(ctx, runID, "completed", nil); err != nil {
		log.Printf("distillation policy run complete: %v", err)
	}
	log.Printf("distillation policy %d triggered (%s) tenant=%s prefix=%s count=%d",
		p.ID, reason, p.TenantID, p.PathPrefix, st.MemoryCount)
}

func (e *DistillationPolicyEngine) beginRun(ctx context.Context, p model.DistillationPolicy, sourceCount int) (int64, error) {
	var runID int64
	err := e.db.QueryRow(ctx, `
		INSERT INTO distillation_runs (policy_id, tenant_id, path_prefix, status, source_count)
		VALUES ($1, $2::uuid, $3, 'running', $4)
		RETURNING id`, p.ID, p.TenantID, p.PathPrefix, sourceCount).Scan(&runID)
	return runID, err
}

func (e *DistillationPolicyEngine) completeRun(ctx context.Context, runID int64, status string, errMsg *string) error {
	_, err := e.db.Exec(ctx, `
		UPDATE distillation_runs
		SET status = $2,
		    error_message = $3,
		    completed_at = NOW()
		WHERE id = $1`, runID, status, errMsg)
	return err
}

// RecordRunFailure marks a run as failed (used when distillation errors are detected).
func (e *DistillationPolicyEngine) RecordRunFailure(ctx context.Context, runID int64, msg string) error {
	if e == nil || e.db == nil {
		return errors.New("policy engine not configured")
	}
	return e.completeRun(ctx, runID, "failed", &msg)
}

func (e *DistillationPolicyEngine) markPolicyTriggered(ctx context.Context, policyID int64) error {
	_, err := e.db.Exec(ctx, `
		UPDATE distillation_policies
		SET last_triggered_at = NOW(), updated_at = NOW()
		WHERE id = $1`, policyID)
	return err
}

type policyScanner interface {
	Scan(dest ...any) error
}

func scanDistillationPolicy(rows policyScanner) (model.DistillationPolicy, error) {
	var p model.DistillationPolicy
	var maxAgeSecs *int
	err := rows.Scan(
		&p.ID, &p.TenantID, &p.Name, &p.PathPrefix, &p.Enabled,
		&p.CountThreshold, &p.MinIntervalSecs, &maxAgeSecs, &p.LastTriggeredAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return p, err
	}
	p.MaxAgeSecs = maxAgeSecs
	return p, nil
}

// InsertPolicyForTest creates a policy row (integration tests).
func InsertPolicyForTest(ctx context.Context, db workerDB, tenantID string, req model.CreateDistillationPolicyRequest) (model.DistillationPolicy, error) {
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	count := req.CountThreshold
	if count < 1 {
		count = 10
	}
	interval := req.MinIntervalSecs
	if interval < 0 {
		interval = 300
	}
	path := strings.TrimSpace(req.PathPrefix)
	if path == "" {
		path = "root"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "default"
	}
	var p model.DistillationPolicy
	err := db.QueryRow(ctx, `
		INSERT INTO distillation_policies (
			tenant_id, name, path_prefix, enabled, count_threshold, min_interval_secs, max_age_secs
		) VALUES ($1::uuid, $2, $3::ltree, $4, $5, $6, $7)
		RETURNING id, tenant_id::text, name, path_prefix::text, enabled,
		          count_threshold, min_interval_secs, max_age_secs, last_triggered_at,
		          created_at, updated_at`,
		tenantID, name, path, enabled, count, interval, req.MaxAgeSecs,
	).Scan(
		&p.ID, &p.TenantID, &p.Name, &p.PathPrefix, &p.Enabled,
		&p.CountThreshold, &p.MinIntervalSecs, &p.MaxAgeSecs, &p.LastTriggeredAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	return p, err
}

// GetLatestRunForPolicy returns the most recent run for a policy (tests).
func GetLatestRunForPolicy(ctx context.Context, db workerDB, policyID int64) (model.DistillationRun, error) {
	var r model.DistillationRun
	var errMsg *string
	err := db.QueryRow(ctx, `
		SELECT id, policy_id, tenant_id::text, path_prefix, status, error_message,
		       source_count, distilled_id, created_at, completed_at
		FROM distillation_runs
		WHERE policy_id = $1
		ORDER BY id DESC
		LIMIT 1`, policyID).Scan(
		&r.ID, &r.PolicyID, &r.TenantID, &r.PathPrefix, &r.Status, &errMsg,
		&r.SourceCount, &r.DistilledID, &r.CreatedAt, &r.CompletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, err
	}
	r.ErrorMessage = errMsg
	return r, err
}
