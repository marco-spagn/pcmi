package auditchain

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"strconv"
	"strings"
	"time"
)

// Export file format (JSON Lines, application/x-ndjson):
//
//	{"type":"entry", ...Record...}      one per audit row, ascending chain_seq
//	...
//	{"type":"trailer", ...Trailer...}   exactly one, last line
//
// Trailer.ContentSHA256 is sha256 over the exact bytes of every entry line
// including its trailing '\n'. When the server has AUDIT_EXPORT_SIGNING_KEY,
// Trailer.Signature is "hmac-sha256=" + hex(HMAC-SHA256(key, signingPayload)).

const (
	LineTypeEntry   = "entry"
	LineTypeTrailer = "trailer"

	signaturePrefix = "hmac-sha256="
)

// EntryLine is one exported audit row.
type EntryLine struct {
	Type string `json:"type"`
	Record
}

// Trailer summarises and seals an export.
type Trailer struct {
	Type           string `json:"type"`
	Algorithm      string `json:"algorithm"`
	TenantID       string `json:"tenant_id"`
	Count          int64  `json:"count"`
	FirstSeq       int64  `json:"first_seq"`
	LastSeq        int64  `json:"last_seq"`
	AnchorPrevHash string `json:"anchor_prev_hash"`
	HeadHash       string `json:"head_hash"`
	ChainValid     bool   `json:"chain_valid"`
	Complete       bool   `json:"complete"`
	NextFromSeq    int64  `json:"next_from_seq,omitempty"`
	ContentSHA256  string `json:"content_sha256"`
	ExportedAt     string `json:"exported_at"`
	Signature      string `json:"signature,omitempty"`
}

// signingPayload is the byte string covered by Trailer.Signature. Every field
// that a consumer relies on is included, in a fixed order.
func (t Trailer) signingPayload() []byte {
	return []byte(strings.Join([]string{
		t.Algorithm,
		t.TenantID,
		strconv.FormatInt(t.Count, 10),
		strconv.FormatInt(t.FirstSeq, 10),
		strconv.FormatInt(t.LastSeq, 10),
		t.AnchorPrevHash,
		t.HeadHash,
		strconv.FormatBool(t.ChainValid),
		strconv.FormatBool(t.Complete),
		strconv.FormatInt(t.NextFromSeq, 10),
		t.ContentSHA256,
		t.ExportedAt,
	}, "\n"))
}

func (t Trailer) mac(key string) []byte {
	m := hmac.New(sha256.New, []byte(key))
	_, _ = m.Write(t.signingPayload())
	return m.Sum(nil)
}

// Sign sets t.Signature with key. An empty key clears the signature.
func (t *Trailer) Sign(key string) {
	if key == "" {
		t.Signature = ""
		return
	}
	t.Signature = signaturePrefix + hex.EncodeToString(t.mac(key))
}

// VerifySignature reports whether t.Signature is a valid HMAC for key.
func (t Trailer) VerifySignature(key string) bool {
	if key == "" || !strings.HasPrefix(t.Signature, signaturePrefix) {
		return false
	}
	got, err := hex.DecodeString(strings.TrimPrefix(t.Signature, signaturePrefix))
	if err != nil {
		return false
	}
	return hmac.Equal(got, t.mac(key))
}

// Writer streams an export: entry lines, then one trailer.
type Writer struct {
	w       io.Writer
	content hash.Hash
	v       *Verifier
	t       Trailer
}

// NewWriter starts an export for tenantID onto w.
func NewWriter(w io.Writer, tenantID string) *Writer {
	return &Writer{
		w:       w,
		content: sha256.New(),
		v:       NewVerifier(tenantID),
		t:       Trailer{Type: LineTypeTrailer, Algorithm: Algorithm, TenantID: tenantID},
	}
}

// Write appends one record (ascending chain_seq).
func (x *Writer) Write(r Record) error {
	line, err := json.Marshal(EntryLine{Type: LineTypeEntry, Record: r})
	if err != nil {
		return fmt.Errorf("auditchain: marshal entry: %w", err)
	}
	line = append(line, '\n')
	if _, err := x.w.Write(line); err != nil {
		return err
	}
	_, _ = x.content.Write(line)
	if x.t.Count == 0 {
		x.t.FirstSeq = r.ChainSeq
		x.t.AnchorPrevHash = r.PrevHash
	}
	x.t.Count++
	x.v.Add(r)
	return nil
}

// Close writes the trailer. complete reports whether the export reached the
// tenant's chain head; nextFromSeq (when not complete) is where to resume.
// signingKey may be empty (unsigned export).
func (x *Writer) Close(exportedAt time.Time, complete bool, nextFromSeq int64, signingKey string) (Trailer, error) {
	x.t.LastSeq = x.v.HeadSeq()
	x.t.HeadHash = x.v.HeadHash()
	x.t.ChainValid = x.v.Valid()
	x.t.Complete = complete
	if !complete {
		x.t.NextFromSeq = nextFromSeq
	}
	x.t.ContentSHA256 = hex.EncodeToString(x.content.Sum(nil))
	x.t.ExportedAt = exportedAt.UTC().Format(time.RFC3339Nano)
	x.t.Sign(signingKey)
	line, err := json.Marshal(x.t)
	if err != nil {
		return Trailer{}, fmt.Errorf("auditchain: marshal trailer: %w", err)
	}
	if _, err := x.w.Write(append(line, '\n')); err != nil {
		return Trailer{}, err
	}
	return x.t, nil
}

// ExportCheck is the outcome of VerifyExport.
type ExportCheck struct {
	Valid            bool     `json:"valid"`
	Entries          int64    `json:"entries"`
	ChainValid       bool     `json:"chain_valid"`
	FirstBreak       *Break   `json:"first_break,omitempty"`
	ContentHashOK    bool     `json:"content_hash_ok"`
	TrailerMatches   bool     `json:"trailer_matches"`
	SignatureChecked bool     `json:"signature_checked"`
	SignatureOK      bool     `json:"signature_ok"`
	Trailer          *Trailer `json:"trailer,omitempty"`
	Problems         []string `json:"problems,omitempty"`
}

// ErrNoTrailer is returned when an export has no trailer line.
var ErrNoTrailer = errors.New("auditchain: export has no trailer line")

// maxLineBytes bounds a single JSONL line (request/response bodies included).
const maxLineBytes = 16 << 20

// VerifyExport re-verifies an export offline: every row hash and link, the
// content digest, the trailer summary, and (when signingKey is non-empty) the
// HMAC signature. It needs no database access.
func VerifyExport(r io.Reader, signingKey string) (ExportCheck, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxLineBytes)

	var (
		out     ExportCheck
		content = sha256.New()
		v       *Verifier
		trailer *Trailer
	)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		raw := sc.Bytes()
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		if trailer != nil {
			return out, fmt.Errorf("auditchain: line %d: data after trailer", lineNo)
		}
		var head struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			return out, fmt.Errorf("auditchain: line %d: %w", lineNo, err)
		}
		switch head.Type {
		case LineTypeEntry:
			var e EntryLine
			if err := json.Unmarshal(raw, &e); err != nil {
				return out, fmt.Errorf("auditchain: line %d: %w", lineNo, err)
			}
			if v == nil {
				v = NewVerifier(e.TenantID)
			}
			v.Add(e.Record)
			_, _ = content.Write(raw)
			_, _ = content.Write([]byte{'\n'})
			out.Entries++
		case LineTypeTrailer:
			var t Trailer
			if err := json.Unmarshal(raw, &t); err != nil {
				return out, fmt.Errorf("auditchain: line %d: %w", lineNo, err)
			}
			trailer = &t
		default:
			return out, fmt.Errorf("auditchain: line %d: unknown type %q", lineNo, head.Type)
		}
	}
	if err := sc.Err(); err != nil {
		return out, err
	}
	if trailer == nil {
		return out, ErrNoTrailer
	}
	out.Trailer = trailer
	if v == nil {
		v = NewVerifier(trailer.TenantID)
	}

	out.ChainValid = v.Valid()
	out.FirstBreak = v.FirstBreak
	if !out.ChainValid {
		out.Problems = append(out.Problems, "chain: "+v.FirstBreak.Reason+" at chain_seq "+strconv.FormatInt(v.FirstBreak.ChainSeq, 10))
	}

	out.ContentHashOK = hex.EncodeToString(content.Sum(nil)) == trailer.ContentSHA256
	if !out.ContentHashOK {
		out.Problems = append(out.Problems, "content_sha256 does not match entry lines")
	}

	out.TrailerMatches = trailer.Algorithm == Algorithm &&
		trailer.Count == out.Entries &&
		trailer.LastSeq == v.HeadSeq() &&
		trailer.HeadHash == v.HeadHash() &&
		trailer.ChainValid == v.Valid() &&
		(out.Entries == 0 || trailer.FirstSeq == v.FirstSeq)
	if !out.TrailerMatches {
		out.Problems = append(out.Problems, "trailer summary (algorithm/count/first_seq/last_seq/head_hash/chain_valid) does not match entries")
	}

	if signingKey != "" {
		out.SignatureChecked = true
		out.SignatureOK = trailer.VerifySignature(signingKey)
		if !out.SignatureOK {
			out.Problems = append(out.Problems, "trailer signature missing or invalid for the supplied key")
		}
	}

	out.Valid = out.ChainValid && out.ContentHashOK && out.TrailerMatches &&
		(!out.SignatureChecked || out.SignatureOK)
	return out, nil
}
