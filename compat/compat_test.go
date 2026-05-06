package compat

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"regexp"
	"testing"

	goconst "github.com/jgautheron/goconst"
)

// filterIssuesByPath simulates golangci-lint's path-based exclusion.
// golangci-lint calls goconst.Run() with ALL files, then filters
// the returned issues by path patterns from its exclusions config.
func filterIssuesByPath(issues []goconst.Issue, excludePatterns []string) []goconst.Issue {
	var filtered []goconst.Issue
	for _, issue := range issues {
		excluded := false
		for _, pattern := range excludePatterns {
			if matched, _ := regexp.MatchString(pattern, issue.Pos.Filename); matched {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, issue)
		}
	}
	return filtered
}

// TestGolangCILintPipeline_ExcludeTests simulates the full golangci-lint
// pipeline for the most common case: test files excluded via path rules.
//
// This is the scenario from issue #57 — golangci-lint passes all files
// to goconst, then filters out _test.go issues afterward.
func TestGolangCILintPipeline_ExcludeTests(t *testing.T) {
	prodCode := `package nullable
const FalseStr = "false"
func ParseBool(s string) bool {
	switch s {
	case "", "true", "false":
		return true
	}
	return false
}`
	testCode := `package nullable
func TestParseBool() {
	_ = "false"
	_ = "false"
	_ = "false"
	_ = "false"
	_ = "false"
}`

	fset := token.NewFileSet()
	fProd, err := parser.ParseFile(fset, "bool.go", prodCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse bool.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "bool_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse bool_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{fProd, fTest})

	// golangci-lint config: goconst with min-occurrences=4, Call excluded.
	// IgnoreTests is false — golangci-lint handles exclusion at its level.
	cfg := &goconst.Config{
		MinStringLength:    3,
		MinOccurrences:     4,
		MatchWithConstants: true,
		ExcludeTypes: map[goconst.Type]bool{
			goconst.Call: true,
		},
		IgnoreTests: false,
	}

	// Step 1: golangci-lint calls goconst with ALL files
	allIssues, err := goconst.Run([]*ast.File{fProd, fTest}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Step 2: golangci-lint applies path exclusion for test files
	//   exclusions:
	//     rules:
	//       - path: _test\.go
	//         linters: [goconst]
	surviving := filterIssuesByPath(allIssues, []string{`_test\.go`})

	// After filtering, bool.go should NOT be flagged.
	// "false" appears only 1x in production code (case clause),
	// which is below min-occurrences=4.
	for _, issue := range surviving {
		if issue.Str == "false" {
			t.Errorf("false positive after path exclusion: %s:%d has %q with %d occurrences",
				issue.Pos.Filename, issue.Pos.Line, issue.Str, issue.OccurrencesCount)
		}
	}
}

// TestGolangCILintPipeline_ExcludeTests_MatchingConst verifies that
// after path-based exclusion, surviving issues don't reference constants
// that only exist in test files.
func TestGolangCILintPipeline_ExcludeTests_MatchingConst(t *testing.T) {
	prodCode := `package example
func prod() {
	_ = "magic"
	_ = "magic"
}`
	testCode := `package example
const TestMagic = "magic"
func testHelper() {
	_ = "magic"
	_ = "magic"
}`

	fset := token.NewFileSet()
	fProd, err := parser.ParseFile(fset, "lib.go", prodCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse lib.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "lib_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse lib_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{fProd, fTest})

	cfg := &goconst.Config{
		MinStringLength:    3,
		MinOccurrences:     2,
		MatchWithConstants: true,
		ExcludeTypes: map[goconst.Type]bool{
			goconst.Call: true,
		},
	}

	allIssues, err := goconst.Run([]*ast.File{fProd, fTest}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	surviving := filterIssuesByPath(allIssues, []string{`_test\.go`})

	if len(surviving) != 1 {
		t.Fatalf("expected 1 surviving issue, got %d", len(surviving))
	}

	issue := surviving[0]
	if issue.Pos.Filename != "lib.go" {
		t.Errorf("expected issue in lib.go, got %s", issue.Pos.Filename)
	}
	if issue.OccurrencesCount != 2 {
		t.Errorf("OccurrencesCount = %d, want 2", issue.OccurrencesCount)
	}
	// The matching constant should NOT reference TestMagic from the test file.
	if issue.MatchingConst == "TestMagic" {
		t.Errorf("MatchingConst = %q — production issue references test-only constant",
			issue.MatchingConst)
	}
}

// TestGolangCILintPipeline_NoExclusion verifies the baseline: when
// golangci-lint does NOT exclude test files, both scopes are reported
// with their own accurate counts.
func TestGolangCILintPipeline_NoExclusion(t *testing.T) {
	prodCode := `package example
func prod() {
	_ = "shared"
	_ = "shared"
	_ = "shared"
}`
	testCode := `package example
func testHelper() {
	_ = "shared"
	_ = "shared"
}`

	fset := token.NewFileSet()
	fProd, err := parser.ParseFile(fset, "lib.go", prodCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse lib.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "lib_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse lib_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{fProd, fTest})

	cfg := &goconst.Config{
		MinStringLength: 3,
		MinOccurrences:  2,
		ExcludeTypes: map[goconst.Type]bool{
			goconst.Call: true,
		},
	}

	issues, err := goconst.Run([]*ast.File{fProd, fTest}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// No exclusion: both files should have issues with per-scope counts.
	counts := make(map[string]int)
	for _, issue := range issues {
		if issue.Str == "shared" {
			counts[issue.Pos.Filename] = issue.OccurrencesCount
		}
	}

	if counts["lib.go"] != 3 {
		t.Errorf("lib.go OccurrencesCount = %d, want 3", counts["lib.go"])
	}
	if counts["lib_test.go"] != 2 {
		t.Errorf("lib_test.go OccurrencesCount = %d, want 2", counts["lib_test.go"])
	}
}

// TestGolangCILintPipeline_FindDuplicates verifies that duplicate
// constant detection works correctly through the golangci-lint pipeline,
// including path-based exclusion of test files.
func TestGolangCILintPipeline_FindDuplicates(t *testing.T) {
	prodCode := `package example
const ProdConst1 = "dup-val"
const ProdConst2 = "dup-val"
`
	testCode := `package example
const TestConst = "dup-val"
`

	fset := token.NewFileSet()
	fProd, err := parser.ParseFile(fset, "consts.go", prodCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse consts.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "consts_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse consts_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{fProd, fTest})

	cfg := &goconst.Config{
		MinStringLength: 3,
		MinOccurrences:  1,
		FindDuplicates:  true,
	}

	allIssues, err := goconst.Run([]*ast.File{fProd, fTest}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	surviving := filterIssuesByPath(allIssues, []string{`_test\.go`})

	// After filtering test issues, we should see the prod duplicate
	// (ProdConst2 duplicates ProdConst1) but NOT a cross-scope duplicate.
	var dupIssues []goconst.Issue
	for _, issue := range surviving {
		if issue.DuplicateConst != "" {
			dupIssues = append(dupIssues, issue)
		}
	}

	if len(dupIssues) != 1 {
		t.Fatalf("expected 1 duplicate issue after filtering, got %d", len(dupIssues))
	}

	dup := dupIssues[0]
	if dup.Pos.Filename != "consts.go" {
		t.Errorf("duplicate issue in %s, want consts.go", dup.Pos.Filename)
	}
	if dup.DuplicateConst != "ProdConst1" && dup.DuplicateConst != "ProdConst2" {
		t.Errorf("DuplicateConst = %q, want ProdConst1 or ProdConst2", dup.DuplicateConst)
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
