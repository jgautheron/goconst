package compat

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	goconstAPI "github.com/jgautheron/goconst"
)

func TestGolangCIIntegration(t *testing.T) {
	const testCode = `package example

const ExistingConst = "test-const"

func example() {
	// This should be detected as it matches ExistingConst
	str1 := "test-const"
	str2 := "test-const"

	// This should be detected as a duplicate without matching constant
	dup1 := "duplicate"
	dup2 := "duplicate"

	// This should be ignored as it's in a function call
	println("ignored-in-call")
	println("ignored-in-call")

	// This should be ignored as it's too short
	x := "a"
	y := "a"

	// This should be ignored due to IgnoreStrings
	skip := "test-ignore"
	skip2 := "test-ignore"
}
`

	// Parse the test code
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	// Configure exactly as golangci-lint does
	cfg := &goconstAPI.Config{
		IgnoreStrings:     "test-ignore",
		MatchWithConstants: true,
		MinStringLength:    3,
		MinOccurrences:     2,
		ParseNumbers:       false,
		ExcludeTypes: map[goconstAPI.Type]bool{
			goconstAPI.Call: true,
		},
		IgnoreTests: false,
	}

	// Run the analysis
	issues, err := goconstAPI.Run([]*ast.File{f}, fset, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify we get exactly the issues we expect
	expectedIssues := map[string]struct {
		count         int
		matchingConst string
	}{
		"test-const": {2, "ExistingConst"},
		"duplicate":  {2, ""},
	}

	if len(issues) != len(expectedIssues) {
		t.Errorf("Got %d issues, want %d", len(issues), len(expectedIssues))
		for _, issue := range issues {
			t.Logf("Found issue: %q matches constant %q with %d occurrences",
				issue.Str, issue.MatchingConst, issue.OccurrencesCount)
		}
	}

	for _, issue := range issues {
		expected, ok := expectedIssues[issue.Str]
		if !ok {
			t.Errorf("Unexpected issue found: %q", issue.Str)
			continue
		}

		if issue.OccurrencesCount != expected.count {
			t.Errorf("String %q: got %d occurrences, want %d",
				issue.Str, issue.OccurrencesCount, expected.count)
		}

		if issue.MatchingConst != expected.matchingConst {
			t.Errorf("String %q: got matching const %q, want %q",
				issue.Str, issue.MatchingConst, expected.matchingConst)
		}
	}
} 