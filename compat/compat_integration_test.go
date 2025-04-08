package compat

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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
		IgnoreStrings:      []string{"test-ignore"},
		MatchWithConstants: true,
		MinStringLength:    3,
		MinOccurrences:     2,
		ParseNumbers:       false,
		ExcludeTypes: map[goconstAPI.Type]bool{
			goconstAPI.Call: true,
		},
		IgnoreTests: false,
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	// Run the analysis
	issues, err := goconstAPI.Run([]*ast.File{f}, fset, info, cfg)
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

func TestMultipleIgnorePatternsIntegration(t *testing.T) {
	const testCode = `package example

func example() {
	// These should be ignored by different patterns
	foo1 := "foobar"
	foo2 := "foobar"
	
	bar1 := "barbaz"
	bar2 := "barbaz"
	
	// These should be detected
	test1 := "example"
	test2 := "example"
}
`

	// Parse the test code
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	// Configure with multiple ignore patterns
	cfg := &goconstAPI.Config{
		IgnoreStrings:   []string{"foo.+", "bar.+"}, // Multiple patterns
		MinStringLength: 3,
		MinOccurrences:  2,
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	// Run the analysis
	issues, err := goconstAPI.Run([]*ast.File{f}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify that "foobar" and "barbaz" are ignored but "example" is found
	if len(issues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(issues))
		for _, issue := range issues {
			t.Logf("Found issue: %q with %d occurrences",
				issue.Str, issue.OccurrencesCount)
		}
		return
	}

	// The only issue should be "example"
	if issues[0].Str != "example" {
		t.Errorf("Expected to find 'example', got %q", issues[0].Str)
	}
}

// TestConstExpressionCompatibility verifies support for constant expressions
func TestConstExpressionCompatibility(t *testing.T) {
	const testCode = `package example

const (
	Prefix = "domain.com/"
	API = Prefix + "api"
	Web = Prefix + "web"
)

func example() {
	// This should match the constant expression
	path1 := "domain.com/api"
	path2 := "domain.com/api"
	
	// This should also match
	web1 := "domain.com/web"
	web2 := "domain.com/web"
}
`

	// Parse the test code
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	// Configure with constant expression evaluation enabled
	cfg := &goconstAPI.Config{
		MinStringLength:      3,
		MinOccurrences:       2,
		MatchWithConstants:   true,
		EvalConstExpressions: true,
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	// Run the analysis
	issues, err := goconstAPI.Run([]*ast.File{f}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Verify we get exactly the issues we expect
	expectedIssues := map[string]struct {
		count         int
		matchingConst string
	}{
		"domain.com/api": {2, "API"},
		"domain.com/web": {2, "Web"},
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

func checker(fset *token.FileSet) (*types.Checker, *types.Info) {
	cfg := &types.Config{
		Error: func(err error) {},
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	return types.NewChecker(cfg, fset, types.NewPackage("", "example"), info), info
}
