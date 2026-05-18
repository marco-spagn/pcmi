package grpcserver

import (
	"encoding/json"
	"testing"
)

func TestToJSONResponseRoundTrip(t *testing.T) {
	in := map[string]any{"entries": []int{1, 2}, "total": 2}
	resp, err := toJSONResponse(in)
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetJson() == "" {
		t.Fatal("empty json")
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(resp.GetJson()), &out); err != nil {
		t.Fatal(err)
	}
	if int(out["total"].(float64)) != 2 {
		t.Fatalf("total: %v", out["total"])
	}
}

func TestToJSONResponseMarshalError(t *testing.T) {
	bad := map[string]any{"x": make(chan int)}
	_, err := toJSONResponse(bad)
	if err == nil {
		t.Fatal("expected error")
	}
}
