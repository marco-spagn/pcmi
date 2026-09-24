package service

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/marco-spagn/pcmi/internal/auditchain"
)

// AuditChainSource reads a tenant's chained audit rows (see
// repository.AuditRepository.ChainBatch / ChainHead).
type AuditChainSource interface {
	ChainBatch(ctx context.Context, tenantID string, afterSeq, toSeq int64, limit int) ([]auditchain.Record, error)
	ChainHead(ctx context.Context, tenantID string) (int64, string, error)
}

// Audit export limits (rows per request).
const (
	AuditExportDefaultLimit = 10000
	AuditExportMaxLimit     = 50000
	auditChainBatchSize     = 1000
)

// ErrAuditExportRange is returned for an invalid from_seq/to_seq/limit.
var ErrAuditExportRange = errors.New("invalid audit export range: need from_seq >= 1, to_seq = 0 or >= from_seq, 1 <= limit <= 50000")

// AuditService verifies and exports the tamper-evident audit chain.
type AuditService struct {
	src        AuditChainSource
	signingKey string
	now        func() time.Time
}

// NewAuditService builds an AuditService. signingKey (AUDIT_EXPORT_SIGNING_KEY)
// may be empty, in which case exports are unsigned.
func NewAuditService(src AuditChainSource, signingKey string) *AuditService {
	return &AuditService{src: src, signingKey: signingKey, now: time.Now}
}

// SigningEnabled reports whether exports carry an HMAC signature.
func (s *AuditService) SigningEnabled() bool { return s.signingKey != "" }

// AuditVerifyResult is the response of GET /v1/audit/verify.
type AuditVerifyResult struct {
	TenantID   string            `json:"tenant_id"`
	Algorithm  string            `json:"algorithm"`
	Valid      bool              `json:"valid"`
	Checked    int64             `json:"checked"`
	HeadSeq    int64             `json:"head_seq"`
	HeadHash   string            `json:"head_hash"`
	FirstBreak *auditchain.Break `json:"first_break,omitempty"`
	VerifiedAt time.Time         `json:"verified_at"`
}

// Verify recomputes every hash and link of the tenant's audit chain.
func (s *AuditService) Verify(ctx context.Context, tenantID string) (AuditVerifyResult, error) {
	v := auditchain.NewVerifier(tenantID)
	var after int64
	for {
		batch, err := s.src.ChainBatch(ctx, tenantID, after, 0, auditChainBatchSize)
		if err != nil {
			return AuditVerifyResult{}, err
		}
		for _, r := range batch {
			v.Add(r)
			after = r.ChainSeq
		}
		if len(batch) < auditChainBatchSize {
			break
		}
	}
	return AuditVerifyResult{
		TenantID:   tenantID,
		Algorithm:  auditchain.Algorithm,
		Valid:      v.Valid(),
		Checked:    v.Checked,
		HeadSeq:    v.HeadSeq(),
		HeadHash:   v.HeadHash(),
		FirstBreak: v.FirstBreak,
		VerifiedAt: s.now().UTC(),
	}, nil
}

// AuditExportOptions selects the exported slice of the chain.
type AuditExportOptions struct {
	FromSeq int64 // first chain_seq (>= 1)
	ToSeq   int64 // last chain_seq inclusive; 0 = up to the head
	Limit   int   // max rows (1..AuditExportMaxLimit)
}

// Normalize applies defaults and validates the options.
func (o AuditExportOptions) Normalize() (AuditExportOptions, error) {
	if o.FromSeq == 0 {
		o.FromSeq = 1
	}
	if o.Limit == 0 {
		o.Limit = AuditExportDefaultLimit
	}
	if o.FromSeq < 1 || o.ToSeq < 0 || (o.ToSeq > 0 && o.ToSeq < o.FromSeq) ||
		o.Limit < 1 || o.Limit > AuditExportMaxLimit {
		return o, ErrAuditExportRange
	}
	return o, nil
}

// Export writes a JSONL export (entries + sealed trailer) to w.
func (s *AuditService) Export(ctx context.Context, tenantID string, opts AuditExportOptions, w io.Writer) (auditchain.Trailer, error) {
	opts, err := opts.Normalize()
	if err != nil {
		return auditchain.Trailer{}, err
	}
	headSeq, _, err := s.src.ChainHead(ctx, tenantID)
	if err != nil {
		return auditchain.Trailer{}, err
	}
	end := headSeq
	if opts.ToSeq > 0 && opts.ToSeq < end {
		end = opts.ToSeq
	}

	xw := auditchain.NewWriter(w, tenantID)
	after := opts.FromSeq - 1
	written := 0
	for written < opts.Limit && after < end {
		n := auditChainBatchSize
		if rem := opts.Limit - written; rem < n {
			n = rem
		}
		batch, err := s.src.ChainBatch(ctx, tenantID, after, end, n)
		if err != nil {
			return auditchain.Trailer{}, err
		}
		for _, r := range batch {
			if err := xw.Write(r); err != nil {
				return auditchain.Trailer{}, err
			}
			after = r.ChainSeq
			written++
		}
		if len(batch) < n {
			break
		}
	}
	complete := after >= end
	return xw.Close(s.now(), complete, after+1, s.signingKey)
}
