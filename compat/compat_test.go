package compat

import (
	"go/token"
	"testing"

	goconstAPI "github.com/jgautheron/goconst"
)

// TestGolangCICompatibility verifies that our API remains compatible
// with how golangci-lint uses it
func TestGolangCICompatibility(t *testing.T) {
  // This test mimics how golangci-lint configures and uses goconst
  // See: https://github.com/golangci/golangci-lint/blob/main/pkg/golinters/goconst/goconst.go
  
  cfg := goconstAPI.Config{
    IgnoreStrings: "test",
    MatchWithConstants: true,
    MinStringLength: 3,
    MinOccurrences: 2,
    ParseNumbers: true,
    NumberMin: 100,
    NumberMax: 1000,
    ExcludeTypes: map[goconstAPI.Type]bool{
      goconstAPI.Call: true,
    },
    IgnoreTests: false,
  }

  // Create a simple test file
  fset := token.NewFileSet()
  
  // Verify that the API call signature matches what golangci-lint expects
  _, err := goconstAPI.Run(nil, fset, &cfg)
  if err != nil {
    // We expect an error since we passed nil files
    // but the important part is that the function signature matches
    t.Log("Expected error from nil files:", err)
  }

  // Verify that the Issue struct has all fields golangci-lint expects
  issue := goconstAPI.Issue{
    Pos: token.Position{},
    OccurrencesCount: 2,
    Str: "test",
    MatchingConst: "TEST",
  }

  // Verify we can access all fields golangci-lint uses
  _ = issue.Pos
  _ = issue.OccurrencesCount
  _ = issue.Str
  _ = issue.MatchingConst
} 