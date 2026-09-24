// Package auditchain implements the tamper-evident hash chain that protects
// audit_log (migration 027_audit_hash_chain.sql).
//
// Each tenant has its own chain. For row n:
//
//	row_hash(n) = hex(sha256(prev_hash(n) + "\n" + Canonical(row n)))
//	prev_hash(n) = row_hash(n-1), or GenesisHash for n = 1
//
// Canonical must stay byte-for-byte identical to the SQL function
// audit_log_canonical; the integration test in this package asserts that.
// The package is pure (no database, no HTTP) so the same code verifies the
// chain server-side (GET /v1/audit/verify) and offline (an export file).
package auditchain

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// Algorithm identifies the chain construction and canonical format.
const Algorithm = "sha256-chain-v1"

// GenesisHash is prev_hash of the first row in every tenant chain.
var GenesisHash = strings.Repeat("0", 64)

// fieldSep separates canonical fields (ASCII unit separator, U+001F).
const fieldSep = "\x1f"

// Fields holds the protected columns of one audit row in their canonical
// text form, exactly as PostgreSQL renders them (see ChainSelectColumns).
// Nullable columns use "" for NULL.
type Fields struct {
	TenantID     string `json:"tenant_id"`
	ChainSeq     int64  `json:"chain_seq"`
	APIKeyID     string `json:"api_key_id"`
	EventType    string `json:"event_type"`
	Path         string `json:"path"`
	Method       string `json:"method"`
	StatusCode   int    `json:"status_code"`
	RequestBody  string `json:"request_body"`  // jsonb::text, "" when NULL
	ResponseBody string `json:"response_body"` // jsonb::text, "" when NULL
	IPAddress    string `json:"ip_address"`    // host(inet), "" when NULL
	UserAgent    string `json:"user_agent"`
	CreatedAt    string `json:"created_at"` // UTC, YYYY-MM-DDTHH:MM:SS.ffffffZ
}

// Record is one chained audit row.
type Record struct {
	Fields
	ID       int64  `json:"id"`
	PrevHash string `json:"prev_hash"`
	RowHash  string `json:"row_hash"`
}

// Canonical returns the canonical text of f (mirror of SQL audit_log_canonical).
func Canonical(f Fields) string {
	return strings.Join([]string{
		"v1",
		f.TenantID,
		strconv.FormatInt(f.ChainSeq, 10),
		f.APIKeyID,
		f.EventType,
		f.Path,
		f.Method,
		strconv.Itoa(f.StatusCode),
		f.RequestBody,
		f.ResponseBody,
		f.IPAddress,
		f.UserAgent,
		f.CreatedAt,
	}, fieldSep)
}

// Hash returns hex(sha256(prev + "\n" + canonical)) (mirror of SQL audit_log_hash).
func Hash(prevHash, canonical string) string {
	sum := sha256.Sum256([]byte(prevHash + "\n" + canonical))
	return hex.EncodeToString(sum[:])
}

// Break reasons reported by Verifier.
const (
	ReasonSequenceGap     = "sequence_gap"
	ReasonLinkMismatch    = "link_mismatch"
	ReasonHashMismatch    = "hash_mismatch"
	ReasonGenesisMismatch = "genesis_mismatch"
	ReasonTenantMismatch  = "tenant_mismatch"
)

// Break describes the first point where a chain stops verifying.
type Break struct {
	ChainSeq int64  `json:"chain_seq"`
	ID       int64  `json:"id,omitempty"`
	Reason   string `json:"reason"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

// Verifier checks records fed in ascending chain_seq order. It stops
// asserting at the first break; later records are still counted.
type Verifier struct {
	tenantID string
	started  bool
	lastSeq  int64
	lastHash string

	FirstSeq   int64
	Checked    int64
	FirstBreak *Break
}

// NewVerifier returns a verifier for one tenant's chain. The first record may
// start at any chain_seq (partial export); if it is seq 1 its prev_hash must
// be GenesisHash, otherwise its prev_hash is taken as the trusted anchor.
func NewVerifier(tenantID string) *Verifier {
	return &Verifier{tenantID: tenantID}
}

// Add verifies r against the chain so far and reports whether the chain is
// still intact after it.
func (v *Verifier) Add(r Record) bool {
	v.Checked++
	if v.FirstBreak != nil {
		return false
	}
	if !v.started {
		v.FirstSeq = r.ChainSeq
	}
	switch {
	case v.tenantID != "" && r.TenantID != v.tenantID:
		v.fail(r, ReasonTenantMismatch, v.tenantID, r.TenantID)
	case !v.started && r.ChainSeq == 1 && r.PrevHash != GenesisHash:
		v.fail(r, ReasonGenesisMismatch, GenesisHash, r.PrevHash)
	case v.started && r.ChainSeq != v.lastSeq+1:
		v.fail(r, ReasonSequenceGap, strconv.FormatInt(v.lastSeq+1, 10), strconv.FormatInt(r.ChainSeq, 10))
	case v.started && r.PrevHash != v.lastHash:
		v.fail(r, ReasonLinkMismatch, v.lastHash, r.PrevHash)
	default:
		if want := Hash(r.PrevHash, Canonical(r.Fields)); want != r.RowHash {
			v.fail(r, ReasonHashMismatch, want, r.RowHash)
		}
	}
	v.started = true
	v.lastSeq = r.ChainSeq
	v.lastHash = r.RowHash
	return v.FirstBreak == nil
}

// Valid reports whether every record added so far verified.
func (v *Verifier) Valid() bool { return v.FirstBreak == nil }

// HeadSeq returns the chain_seq of the last record added (0 if none).
func (v *Verifier) HeadSeq() int64 { return v.lastSeq }

// HeadHash returns the row_hash of the last record added ("" if none).
func (v *Verifier) HeadHash() string { return v.lastHash }

func (v *Verifier) fail(r Record, reason, expected, actual string) {
	v.FirstBreak = &Break{ChainSeq: r.ChainSeq, ID: r.ID, Reason: reason, Expected: expected, Actual: actual}
}
