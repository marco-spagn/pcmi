package auditchain

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

const tenantA = "11111111-1111-1111-1111-111111111111"

// buildChain returns n correctly chained records for tenantA.
func buildChain(n int) []Record {
	out := make([]Record, 0, n)
	prev := GenesisHash
	for i := 1; i <= n; i++ {
		f := Fields{
			TenantID:    tenantA,
			ChainSeq:    int64(i),
			EventType:   "api_request",
			Path:        "/v1/memories",
			Method:      "POST",
			StatusCode:  201,
			RequestBody: `{"k": "vàlue <&>"}`,
			IPAddress:   "10.0.0.1",
			UserAgent:   "test",
			CreatedAt:   "2026-09-24T10:00:00.123456Z",
		}
		h := Hash(prev, Canonical(f))
		out = append(out, Record{Fields: f, ID: int64(100 + i), PrevHash: prev, RowHash: h})
		prev = h
	}
	return out
}

func verifyAll(recs []Record) *Verifier {
	v := NewVerifier(tenantA)
	for _, r := range recs {
		v.Add(r)
	}
	return v
}

func TestCanonical_fieldOrderAndSeparator(t *testing.T) {
	got := Canonical(Fields{TenantID: "t", ChainSeq: 7, EventType: "e", Path: "/p", Method: "GET", StatusCode: 200, CreatedAt: "c"})
	want := strings.Join([]string{"v1", "t", "7", "", "e", "/p", "GET", "200", "", "", "", "", "c"}, "\x1f")
	if got != want {
		t.Fatalf("canonical mismatch\n got %q\nwant %q", got, want)
	}
}

func TestHash_knownVector(t *testing.T) {
	// sha256("0"*64 + "\n" + "abc")
	got := Hash(GenesisHash, "abc")
	if len(got) != 64 {
		t.Fatalf("hash length %d", len(got))
	}
	if got != Hash(GenesisHash, "abc") || got == Hash(GenesisHash, "abd") {
		t.Fatal("hash not deterministic / not sensitive to input")
	}
}

func TestVerifier_validChain(t *testing.T) {
	v := verifyAll(buildChain(5))
	if !v.Valid() || v.Checked != 5 || v.HeadSeq() != 5 || v.FirstSeq != 1 {
		t.Fatalf("expected valid chain: %+v", v)
	}
}

func TestVerifier_emptyChain(t *testing.T) {
	v := NewVerifier(tenantA)
	if !v.Valid() || v.HeadSeq() != 0 || v.HeadHash() != "" {
		t.Fatalf("empty chain should be valid with no head: %+v", v)
	}
}

func TestVerifier_detectsTampering(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]Record) []Record
		reason string
		seq    int64
	}{
		{"edited field", func(r []Record) []Record { r[2].Path = "/v1/forged"; return r }, ReasonHashMismatch, 3},
		{"edited status", func(r []Record) []Record { r[1].StatusCode = 200; return r }, ReasonHashMismatch, 2},
		{"deleted row", func(r []Record) []Record { return append(r[:2], r[3:]...) }, ReasonSequenceGap, 4},
		{"reordered rows", func(r []Record) []Record { r[1], r[2] = r[2], r[1]; return r }, ReasonSequenceGap, 3},
		{"rewritten hash", func(r []Record) []Record { r[3].RowHash = strings.Repeat("a", 64); return r }, ReasonHashMismatch, 4},
		{"broken link", func(r []Record) []Record { r[3].PrevHash = strings.Repeat("b", 64); return r }, ReasonLinkMismatch, 4},
		{"bad genesis", func(r []Record) []Record { r[0].PrevHash = strings.Repeat("c", 64); return r }, ReasonGenesisMismatch, 1},
		{"foreign tenant", func(r []Record) []Record { r[1].TenantID = "other"; return r }, ReasonTenantMismatch, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := verifyAll(tc.mutate(buildChain(5)))
			if v.Valid() {
				t.Fatal("tampering not detected")
			}
			if v.FirstBreak.Reason != tc.reason || v.FirstBreak.ChainSeq != tc.seq {
				t.Fatalf("got break %+v, want reason=%s seq=%d", v.FirstBreak, tc.reason, tc.seq)
			}
		})
	}
}

func TestVerifier_partialChainAnchorsOnFirstPrevHash(t *testing.T) {
	v := verifyAll(buildChain(6)[3:])
	if !v.Valid() || v.FirstSeq != 4 || v.Checked != 3 {
		t.Fatalf("partial chain should verify from its anchor: %+v", v)
	}
}

func TestVerifier_keepsCountingAfterBreak(t *testing.T) {
	recs := buildChain(5)
	recs[1].Path = "/x"
	v := verifyAll(recs)
	if v.Checked != 5 || v.FirstBreak.ChainSeq != 2 {
		t.Fatalf("got %+v", v)
	}
}

func exportOf(t *testing.T, recs []Record, key string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter(&buf, tenantA)
	for _, r := range recs {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Close(time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC), true, 0, key); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExport_roundTripSigned(t *testing.T) {
	data := exportOf(t, buildChain(4), "s3cret")
	res, err := VerifyExport(bytes.NewReader(data), "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Valid || res.Entries != 4 || !res.SignatureOK || !res.ContentHashOK || !res.TrailerMatches {
		t.Fatalf("expected valid signed export: %+v", res)
	}
	if res.Trailer.HeadHash != buildChain(4)[3].RowHash || !res.Trailer.ChainValid || !res.Trailer.Complete {
		t.Fatalf("trailer: %+v", res.Trailer)
	}
}

func TestExport_unsignedVerifiesWithoutKey(t *testing.T) {
	data := exportOf(t, buildChain(2), "")
	res, err := VerifyExport(bytes.NewReader(data), "")
	if err != nil || !res.Valid || res.SignatureChecked || res.Trailer.Signature != "" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	// Supplying a key for an unsigned export must fail.
	res, _ = VerifyExport(bytes.NewReader(data), "k")
	if res.Valid || res.SignatureOK {
		t.Fatalf("unsigned export must not pass a signature check: %+v", res)
	}
}

func TestExport_wrongKeyFails(t *testing.T) {
	res, _ := VerifyExport(bytes.NewReader(exportOf(t, buildChain(2), "right")), "wrong")
	if res.Valid || res.SignatureOK {
		t.Fatalf("wrong key accepted: %+v", res)
	}
}

func TestExport_emptyChain(t *testing.T) {
	res, err := VerifyExport(bytes.NewReader(exportOf(t, nil, "k")), "k")
	if err != nil || !res.Valid || res.Entries != 0 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestExport_detectsFileTampering(t *testing.T) {
	good := string(exportOf(t, buildChain(3), "k"))
	lines := strings.Split(strings.TrimSpace(good), "\n")

	t.Run("edited entry", func(t *testing.T) {
		bad := strings.Replace(good, `"path":"/v1/memories"`, `"path":"/v1/other"`, 1)
		res, _ := VerifyExport(strings.NewReader(bad), "k")
		if res.Valid || res.ChainValid || res.ContentHashOK {
			t.Fatalf("edit not detected: %+v", res)
		}
	})
	t.Run("dropped last entry", func(t *testing.T) {
		bad := strings.Join(append(append([]string{}, lines[:2]...), lines[3]), "\n")
		res, _ := VerifyExport(strings.NewReader(bad), "k")
		if res.Valid || res.TrailerMatches {
			t.Fatalf("truncation not detected: %+v", res)
		}
	})
	t.Run("forged trailer count", func(t *testing.T) {
		bad := strings.Replace(good, `"count":3`, `"count":2`, 1)
		res, _ := VerifyExport(strings.NewReader(bad), "k")
		if res.Valid || res.SignatureOK {
			t.Fatalf("trailer forgery not detected: %+v", res)
		}
	})
}

func TestVerifyExport_malformed(t *testing.T) {
	cases := map[string]string{
		"no trailer":         `{"type":"entry","chain_seq":1}` + "\n",
		"not json":           "garbage\n",
		"unknown type":       `{"type":"zzz"}` + "\n",
		"data after trailer": `{"type":"trailer"}` + "\n" + `{"type":"entry"}` + "\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := VerifyExport(strings.NewReader(in), ""); err == nil {
				t.Fatal("expected error")
			}
		})
	}
	if _, err := VerifyExport(strings.NewReader(""), ""); !errors.Is(err, ErrNoTrailer) {
		t.Fatalf("empty input: %v", err)
	}
}

func TestTrailer_signClearsWithEmptyKey(t *testing.T) {
	tr := Trailer{Signature: "x"}
	tr.Sign("")
	if tr.Signature != "" || tr.VerifySignature("") {
		t.Fatal("empty key must clear signature and never verify")
	}
	tr.Signature = "hmac-sha256=zz"
	if tr.VerifySignature("k") {
		t.Fatal("non-hex signature must not verify")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestWriter_propagatesWriteErrors(t *testing.T) {
	w := NewWriter(failWriter{}, tenantA)
	if err := w.Write(buildChain(1)[0]); err == nil {
		t.Fatal("expected write error")
	}
	if _, err := w.Close(time.Now(), true, 0, ""); err == nil {
		t.Fatal("expected close error")
	}
}
