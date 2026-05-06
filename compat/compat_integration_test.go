package compat

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	goconst "github.com/jgautheron/goconst"
)

// TestGolangCILintConfig_IgnoreStrings verifies that multiple ignore
// string patterns work when configured the way golangci-lint does.
func TestGolangCILintConfig_IgnoreStrings(t *testing.T) {
	const code = `package example

func example() {
	foo1 := "foobar"
	foo2 := "foobar"

	bar1 := "barbaz"
	bar2 := "barbaz"

	test1 := "example"
	test2 := "example"
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	cfg := &goconst.Config{
		IgnoreStrings:   []string{"foo.+", "bar.+"},
		MinStringLength: 3,
		MinOccurrences:  2,
	}

	issues, err := goconst.Run([]*ast.File{f}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Str != "example" {
		t.Errorf("expected 'example', got %q", issues[0].Str)
	}
}

// TestGolangCILintConfig_ExcludeCallAndIgnore verifies combined config:
// ExcludeTypes + IgnoreStrings + MatchWithConstants, which is the
// typical golangci-lint setup.
func TestGolangCILintConfig_ExcludeCallAndIgnore(t *testing.T) {
	const code = `package example

const ExistingConst = "test-const"

func example() {
	str1 := "test-const"
	str2 := "test-const"

	dup1 := "duplicate"
	dup2 := "duplicate"

	println("ignored-in-call")
	println("ignored-in-call")

	x := "a"
	y := "a"

	skip := "test-ignore"
	skip2 := "test-ignore"
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	cfg := &goconst.Config{
		IgnoreStrings:      []string{"test-ignore"},
		MatchWithConstants: true,
		MinStringLength:    3,
		MinOccurrences:     2,
		ExcludeTypes: map[goconst.Type]bool{
			goconst.Call: true,
		},
	}

	issues, err := goconst.Run([]*ast.File{f}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	expected := map[string]string{
		"test-const": "ExistingConst",
		"duplicate":  "",
	}

	if len(issues) != len(expected) {
		t.Errorf("got %d issues, want %d", len(issues), len(expected))
		for _, issue := range issues {
			t.Logf("  %q (const=%q, count=%d)", issue.Str, issue.MatchingConst, issue.OccurrencesCount)
		}
	}

	for _, issue := range issues {
		wantConst, ok := expected[issue.Str]
		if !ok {
			t.Errorf("unexpected issue for %q", issue.Str)
			continue
		}
		if issue.MatchingConst != wantConst {
			t.Errorf("%q: MatchingConst = %q, want %q", issue.Str, issue.MatchingConst, wantConst)
		}
		if issue.OccurrencesCount != 2 {
			t.Errorf("%q: OccurrencesCount = %d, want 2", issue.Str, issue.OccurrencesCount)
		}
	}
}

// TestGolangCILintConfig_ConstExpressions verifies that constant
// expression evaluation works through the golangci-lint API path.
func TestGolangCILintConfig_ConstExpressions(t *testing.T) {
	const code = `package example

const (
	Prefix = "domain.com/"
	API = Prefix + "api"
	Web = Prefix + "web"
)

func example() {
	path1 := "domain.com/api"
	path2 := "domain.com/api"

	web1 := "domain.com/web"
	web2 := "domain.com/web"
}
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	cfg := &goconst.Config{
		MinStringLength:      3,
		MinOccurrences:       2,
		MatchWithConstants:   true,
		EvalConstExpressions: true,
	}

	issues, err := goconst.Run([]*ast.File{f}, fset, info, cfg)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	expected := map[string]string{
		"domain.com/api": "API",
		"domain.com/web": "Web",
	}

	if len(issues) != len(expected) {
		t.Errorf("got %d issues, want %d", len(issues), len(expected))
	}

	for _, issue := range issues {
		wantConst, ok := expected[issue.Str]
		if !ok {
			t.Errorf("unexpected issue for %q", issue.Str)
			continue
		}
		if issue.MatchingConst != wantConst {
			t.Errorf("%q: MatchingConst = %q, want %q", issue.Str, issue.MatchingConst, wantConst)
		}
	}
}
