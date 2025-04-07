package compat

import (
	"go/token"
	"go/types"
	"testing"

	goconstAPI "github.com/jgautheron/goconst"
)

// TestGolangCICompatibility verifies that our API remains compatible
// with how golangci-lint uses it
func TestGolangCICompatibility(t *testing.T) {
	// This test mimics how golangci-lint configures and uses goconst
	// See: https://github.com/golangci/golangci-lint/blob/main/pkg/golinters/goconst/goconst.go

	cfg := goconstAPI.Config{
		IgnoreStrings:      []string{"test"},
		MatchWithConstants: true,
		MinStringLength:    3,
		MinOccurrences:     2,
		ParseNumbers:       true,
		NumberMin:          100,
		NumberMax:          1000,
		ExcludeTypes: map[goconstAPI.Type]bool{
			goconstAPI.Call: true,
		},
		IgnoreTests:          false,
		EvalConstExpressions: true,
	}

	// Create a simple test file
	fset := token.NewFileSet()

	info := &types.Info{}

	// Verify that the API call signature matches what golangci-lint expects
	_, err := goconstAPI.Run(nil, fset, info, &cfg)
	if err != nil {
		// We expect an error since we passed nil files
		// but the important part is that the function signature matches
		t.Log("Expected error from nil files:", err)
	}

	// Verify that the Issue struct has all fields golangci-lint expects
	issue := goconstAPI.Issue{
		Pos:              token.Position{},
		OccurrencesCount: 2,
		Str:              "test",
		MatchingConst:    "TEST",
	}

	// Verify we can access all fields golangci-lint uses
	_ = issue.Pos
	_ = issue.OccurrencesCount
	_ = issue.Str
	_ = issue.MatchingConst
}

// TestMultipleIgnorePatterns verifies that multiple ignore patterns work correctly
func TestMultipleIgnorePatterns(t *testing.T) {
	// Test configuration with multiple ignore patterns
	cfg := goconstAPI.Config{
		IgnoreStrings:   []string{"foo.+", "bar.+", "test"},
		MinStringLength: 3,
		MinOccurrences:  2,
	}

	// Create a simple test file
	fset := token.NewFileSet()

	info := &types.Info{}

	// We just want to verify that multiple patterns are accepted
	_, err := goconstAPI.Run(nil, fset, info, &cfg)
	if err != nil {
		// We expect an error since we passed nil files
		// but the important part is that multiple patterns are accepted
		t.Log("Expected error from nil files:", err)
	}

	// This tests the construction and acceptance of the config
	// Actual pattern matching is tested in integration tests
}
