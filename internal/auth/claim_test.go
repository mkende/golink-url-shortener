package auth

import (
	"encoding/json"
	"testing"
)

// TestClaimIsTrue covers the truthy-claim parsing used for email verification,
// including the boolean and string-valued forms emitted by different providers.
func TestClaimIsTrue(t *testing.T) {
	cases := []struct {
		name string
		raw  string // raw JSON, or "" for an absent claim
		want bool
	}{
		{"absent", "", false},
		{"bool true", "true", true},
		{"bool false", "false", false},
		{"string true", `"true"`, true},
		{"string TRUE", `"TRUE"`, true},
		{"string false", `"false"`, false},
		{"string empty", `""`, false},
		{"number", "1", false},
		{"null", "null", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw json.RawMessage
			if tc.raw != "" {
				raw = json.RawMessage(tc.raw)
			}
			if got := claimIsTrue(raw); got != tc.want {
				t.Errorf("claimIsTrue(%q) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}
