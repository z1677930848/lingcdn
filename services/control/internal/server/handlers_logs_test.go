package server

import "testing"

func TestParseLogStatusFilter(t *testing.T) {
	tests := []struct {
		in   any
		want map[string]any
	}{
		{"2xx", map[string]any{"gte": 200, "lte": 299}},
		{"4xx", map[string]any{"gte": 400, "lte": 499}},
		{404, map[string]any{"gte": 404, "lte": 404}},
		{float64(500), map[string]any{"gte": 500, "lte": 500}},
		{"", nil},
		{"bad", nil},
	}
	for _, tc := range tests {
		got := parseLogStatusFilter(tc.in)
		if tc.want == nil {
			if got != nil {
				t.Fatalf("parseLogStatusFilter(%v) = %v, want nil", tc.in, got)
			}
			continue
		}
		if got == nil || got["gte"] != tc.want["gte"] || got["lte"] != tc.want["lte"] {
			t.Fatalf("parseLogStatusFilter(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
