package links_test

import (
	"strings"
	"testing"

	"github.com/mkende/golink-url-shortener/internal/links"
)

func TestGenerateQuickName(t *testing.T) {
	t.Parallel()
	const validChars = "abcdefghijklmnopqrstuvwxyz0123456789"

	tests := []struct {
		name       string
		length     int
		wantLength int
	}{
		{name: "default-length", length: 6, wantLength: 6},
		{name: "longer", length: 10, wantLength: 10},
		{name: "minimum", length: 4, wantLength: 4},
		{name: "zero-uses-default", length: 0, wantLength: 6},
		{name: "negative-uses-default", length: -1, wantLength: 6},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := links.GenerateQuickName(tc.length)
			if err != nil {
				t.Fatalf("GenerateQuickName(%d) unexpected error: %v", tc.length, err)
			}
			if len(got) != tc.wantLength {
				t.Errorf("GenerateQuickName(%d) len = %d, want %d (got %q)", tc.length, len(got), tc.wantLength, got)
			}
			for _, c := range got {
				if !strings.ContainsRune(validChars, c) {
					t.Errorf("GenerateQuickName(%d) returned invalid char %q in %q", tc.length, c, got)
				}
			}
		})
	}
}

func TestGenerateQuickName_Uniqueness(t *testing.T) {
	t.Parallel()
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		name, err := links.GenerateQuickName(6)
		if err != nil {
			t.Fatalf("GenerateQuickName error: %v", err)
		}
		seen[name] = true
	}
	// With 36^6 ~= 2 billion possibilities, 100 samples should be unique.
	if len(seen) < 95 {
		t.Errorf("expected near-unique names; got %d unique out of 100", len(seen))
	}
}
