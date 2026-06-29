package model

import "testing"

func TestNormalizeLinkType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty defaults to related", "", DefaultLinkType, false},
		{"whitespace defaults to related", "   ", DefaultLinkType, false},
		{"causal", "causal", "causal", false},
		{"temporal", "temporal", "temporal", false},
		{"contradicts", "contradicts", "contradicts", false},
		{"supports", "supports", "supports", false},
		{"related", "related", "related", false},
		{"uppercase normalized", "CAUSAL", "causal", false},
		{"mixed case with spaces", "  Supports ", "supports", false},
		{"duplicate not user-assignable", "duplicate", "", true},
		{"unknown rejected", "caused_by", "", true},
		{"injection attempt rejected", "related; DROP", "", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeLinkType(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeLinkType(%q): expected error, got %q", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeLinkType(%q): unexpected error %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeLinkType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
