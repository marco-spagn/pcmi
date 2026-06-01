package config

import "testing"

func TestEnvBool(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fallback bool
		want     bool
		setEnv   bool // if false, the env var is not set
	}{
		{name: "not set, fallback false", envValue: "", fallback: false, want: false, setEnv: false},
		{name: "not set, fallback true", envValue: "", fallback: true, want: true, setEnv: false},
		{name: "true", envValue: "true", want: true, setEnv: true},
		{name: "1", envValue: "1", want: true, setEnv: true},
		{name: "yes", envValue: "yes", want: true, setEnv: true},
		{name: "false", envValue: "false", want: false, setEnv: true},
		{name: "0", envValue: "0", want: false, setEnv: true},
		{name: "no", envValue: "no", want: false, setEnv: true},
		{name: "garbage", envValue: "garbage", want: false, setEnv: true},
		{name: "TRUE", envValue: "TRUE", want: false, setEnv: true}, // case-sensitive: only lowercase "true"
		{name: "whitespace", envValue: "  true  ", want: true, setEnv: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_BOOL_" + tt.name
			if tt.setEnv {
				t.Setenv(key, tt.envValue)
			} else {
				// Ensure the key is not set (t.Setenv restores the previous value).
				t.Setenv(key, "")
			}
			got := envBool(key, tt.fallback)
			if got != tt.want {
				t.Errorf("envBool(%q, %v) = %v, want %v (env=%q)", key, tt.fallback, got, tt.want, tt.envValue)
			}
		})
	}
}

func TestParseDedupModeConfig(t *testing.T) {
	tests := []struct {
		input    string
		want     string
		wantErr  bool
	}{
		{"", "none", false},
		{"none", "none", false},
		{"skip", "skip", false},
		{"link", "link", false},
		{"merge", "merge", false},
		{"NONE", "none", false},
		{"  link  ", "link", false},
		{"invalid", "", true},
		{"unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDedupModeConfig(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDedupModeConfig(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseDedupModeConfig(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
