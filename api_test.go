package goconst

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		config         *Config
		expectedIssues int
	}{
		{
			name: "basic string duplication",
			code: `package example
func example() {
	a := "duplicate"
	b := "duplicate"
}`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
			},
			expectedIssues: 1,
		},
		{
			name: "number duplication",
			code: `package example
func example() {
	a := 12345
	b := 12345
}`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
				ParseNumbers:    true,
			},
			expectedIssues: 1,
		},
		{
			name: "duplicate consts",
			code: `package example
const ConstA = "test"
func example() {
	const ConstB = "test"
}`,
			config: &Config{
				FindDuplicates: true,
			},
			expectedIssues: 1,
		},
		{
			name: "duplicate computed consts",
			code: `package example
const ConstA = "te"
const Test = "test"
func example() {
	const ConstB = ConstA + "st"
}`,
			config: &Config{
				FindDuplicates:       true,
				EvalConstExpressions: true,
			},
			expectedIssues: 1,
		},
		{
			name: "string duplication with ignore",
			code: `package example
func example() {
	a := "test"
	b := "test"
	c := "another"
	d := "another"
}`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
				IgnoreStrings:   []string{"test"},
			},
			expectedIssues: 1, // Only "another" should be reported
		},
		{
			name: "number filtering by min value",
			code: `package example
func example() {
	a := 100
	b := 100
	c := 200
	d := 200
}`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
				ParseNumbers:    true,
				NumberMin:       150,
			},
			expectedIssues: 1, // Only 200 should be reported
		},
		{
			name: "number filtering by max value",
			code: `package example
func example() {
	a := 1000
	b := 1000
	c := 5000
	d := 5000
}`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
				ParseNumbers:    true,
				NumberMax:       2000,
			},
			expectedIssues: 1, // Only 1000 should be reported
		},
		{
			name: "min occurrences filtering",
			code: `package example
func example() {
	a := "one"
	b := "two"
	c := "three"
	d := "three" // only this repeats
}`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
			},
			expectedIssues: 1, // Only "three" repeats enough times
		},
		{
			name: "min length filtering",
			code: `package example
func example() {
	a := "ab" // too short
	b := "ab" // too short
	c := "long"
	d := "long"
}`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
			},
			expectedIssues: 1, // Only "long" meets the min length
		},
		{
			name: "exclusion by type",
			code: `package example
func example() {
	// Assignment context
	a := "test"
	b := "test"
	
	// Binary expression context (should be excluded)
	if a == "exclude" || b == "exclude" {
		return
	}
}`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
				ExcludeTypes:    map[Type]bool{Binary: true},
			},
			expectedIssues: 1, // Only "test" should be reported, "exclude" is filtered
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the test code
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "example.go", tt.code, 0)
			if err != nil {
				t.Fatalf("Failed to parse test code: %v", err)
			}

			chkr, info := checker(fset)
			_ = chkr.Files([]*ast.File{f})

			issues, err := Run([]*ast.File{f}, fset, info, tt.config)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if len(issues) != tt.expectedIssues {
				t.Errorf("Run() = %v issues, want %v", len(issues), tt.expectedIssues)
				for _, issue := range issues {
					t.Logf("Found issue: %s at %s with %d occurrences",
						issue.Str, issue.Pos.String(), issue.OccurrencesCount)
				}
			}
		})
	}
}

func TestIssueFields(t *testing.T) {
	// Test that issue fields are correctly populated
	code := `package example
const MatchingConst = "match"
func example() {
	a := "match"
	b := "match"
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	config := &Config{
		MinStringLength:    3,
		MinOccurrences:     2,
		MatchWithConstants: true,
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, config)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("Expected 1 issue, got %d", len(issues))
	}

	issue := issues[0]
	if issue.Str != "match" {
		t.Errorf("Issue.Str = %v, want %v", issue.Str, "match")
	}
	if issue.OccurrencesCount != 2 {
		t.Errorf("Issue.OccurrencesCount = %v, want 2", issue.OccurrencesCount)
	}
	if issue.MatchingConst != "MatchingConst" {
		t.Errorf("Issue.MatchingConst = %v, want MatchingConst", issue.MatchingConst)
	}
}

func TestMultipleFilesAnalysis(t *testing.T) {
	// Test analyzing multiple files at once
	tests := []struct {
		name                    string
		code1                   string
		code2                   string
		config                  *Config
		expectedIssues          int
		expectedStr             string
		expectedOccurrenceCount int
	}{
		{
			name: "duplicate strings",
			code1: `package example
func example1() {
	a := "shared"
	b := "shared"
}
`,
			code2: `package example
func example2() {
	c := "shared"
	d := "unique"
}
`,
			config: &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
			},
			expectedIssues:          2, // one per file
			expectedStr:             "shared",
			expectedOccurrenceCount: 3,
		},
		{
			name: "duplicate consts in different files",
			code1: `package example
const ConstA = "shared"
const ConstB = "shared"
`,
			code2: `package example
const (
	ConstC = "shared"
	ConstD = "shared"
	ConstE = "unique"
)`,
			config: &Config{
				FindDuplicates: true,
			},
			expectedIssues:          3,
			expectedStr:             "shared",
			expectedOccurrenceCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			fset := token.NewFileSet()
			f1, err := parser.ParseFile(fset, "file1.go", tt.code1, 0)
			if err != nil {
				t.Fatalf("Failed to parse test code: %v", err)
			}

			f2, err := parser.ParseFile(fset, "file2.go", tt.code2, 0)
			if err != nil {
				t.Fatalf("Failed to parse test code: %v", err)
			}

			chkr, info := checker(fset)
			_ = chkr.Files([]*ast.File{f1, f2})

			issues, err := Run([]*ast.File{f1, f2}, fset, info, tt.config)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			// Should find "shared" appearing 3 times across both files
			if len(issues) != tt.expectedIssues {
				t.Fatalf("Expected %d issue, got %d", tt.expectedIssues, len(issues))
			}

			for _, issue := range issues {
				if issue.Str != tt.expectedStr {
					t.Errorf("Issue.Str = %v, want %v", issue.Str, tt.expectedStr)
				}
				if tt.expectedOccurrenceCount > 0 && issue.OccurrencesCount != tt.expectedOccurrenceCount {
					t.Errorf("Issue.OccurrencesCount = %v, want %d", issue.OccurrencesCount, tt.expectedOccurrenceCount)
				}
			}
		})
	}
}

func TestAllTypesOfContexts(t *testing.T) {
	// Test detection in all contexts (assignment, binary, case, return, call)
	code := `package example
const ExistingConst = "constant"

func allContexts(param string) string {
	// Assignment
	a := "test"
	
	// Binary expression
	if param == "test" {
		// Case statement
		switch param {
		case "test":
			// Function call
			println("test")
			// Return statement
			return "test"
		}
	}
	
	return "other"
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	config := &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
	}
	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, config)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should find "test" in 5 different contexts
	if len(issues) != 1 {
		t.Fatalf("Expected 1 issue, got %d", len(issues))
	}

	issue := issues[0]
	if issue.Str != "test" {
		t.Errorf("Issue.Str = %v, want %v", issue.Str, "test")
	}
	if issue.OccurrencesCount != 5 {
		t.Errorf("Issue.OccurrencesCount = %v, want 5", issue.OccurrencesCount)
	}
}

func TestCompositeLiteralContexts(t *testing.T) {
	code := `package example
type person struct {
	name string
}

func compositeContexts() {
	_ = []string{"repeated literal"}
	_ = map[string]string{
		"first":  "repeated literal",
		"second": "repeated literal",
	}
	_ = person{name: "repeated literal"}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	config := &Config{
		MinStringLength: 3,
		MinOccurrences:  4,
	}
	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, config)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("Expected 1 issue, got %d", len(issues))
	}

	issue := issues[0]
	if issue.Str != "repeated literal" {
		t.Errorf("Issue.Str = %v, want %v", issue.Str, "repeated literal")
	}
	if issue.OccurrencesCount != 4 {
		t.Errorf("Issue.OccurrencesCount = %v, want 4", issue.OccurrencesCount)
	}
}

func TestExcludeByMultipleTypes(t *testing.T) {
	// Test excluding multiple context types
	code := `package example
func multipleContexts() {
	// Assignment
	a := "test"
	b := "test"
	
	// Binary expression
	if a == "other" || b == "other" {
		// Return statement
		return "other"
	}
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	// Test with various exclude combinations
	tests := []struct {
		name           string
		excludeTypes   map[Type]bool
		expectedIssues int
		expectedStrs   []string
	}{
		{
			name:           "exclude none",
			excludeTypes:   map[Type]bool{},
			expectedIssues: 2, // "test" and "other"
			expectedStrs:   []string{"test", "other"},
		},
		{
			name:           "exclude assignment",
			excludeTypes:   map[Type]bool{Assignment: true},
			expectedIssues: 1, // only "other"
			expectedStrs:   []string{"other"},
		},
		{
			name:           "exclude binary and return",
			excludeTypes:   map[Type]bool{Binary: true, Return: true},
			expectedIssues: 1, // only "test"
			expectedStrs:   []string{"test"},
		},
		{
			name:           "exclude all types",
			excludeTypes:   map[Type]bool{Assignment: true, Binary: true, Return: true},
			expectedIssues: 0, // nothing left
			expectedStrs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				MinStringLength: 3,
				MinOccurrences:  2,
				ExcludeTypes:    tt.excludeTypes,
			}
			chkr, info := checker(fset)
			_ = chkr.Files([]*ast.File{f})

			issues, err := Run([]*ast.File{f}, fset, info, config)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if len(issues) != tt.expectedIssues {
				t.Errorf("Run() = %v issues, want %v", len(issues), tt.expectedIssues)
			}

			// Check that we found the expected strings
			foundStrs := make(map[string]bool)
			for _, issue := range issues {
				foundStrs[issue.Str] = true
			}

			for _, expectedStr := range tt.expectedStrs {
				if !foundStrs[expectedStr] {
					t.Errorf("Expected string %q not found", expectedStr)
				}
			}
		})
	}
}

func TestConstExpressions(t *testing.T) {
	// Test detecting and matching string constants derived from expressions
	code := `package example

const (
	Prefix = "example.com/"
	Label1 = Prefix + "some_label"
	Label2 = Prefix + "another_label"
)

func example() {
	// These should match the constants from expressions
	a := "example.com/some_label"
	b := "example.com/some_label"
	
	// This should also match
	web1 := "example.com/another_label"
	web2 := "example.com/another_label"
}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	config := &Config{
		MinStringLength:      3,
		MinOccurrences:       2,
		MatchWithConstants:   true,
		EvalConstExpressions: true,
	}
	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, config)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// We expect issues for both labels
	expectedMatches := map[string]string{
		"example.com/some_label":    "Label1",
		"example.com/another_label": "Label2",
	}

	// Check that we have two issues
	if len(issues) != 2 {
		t.Errorf("Expected 2 issues, got %d", len(issues))
		for _, issue := range issues {
			t.Logf("Found issue: %q matches constant %q with %d occurrences",
				issue.Str, issue.MatchingConst, issue.OccurrencesCount)
		}
		return
	}

	// Check that each string matches the expected constant
	for _, issue := range issues {
		expectedConst, ok := expectedMatches[issue.Str]
		if !ok {
			t.Errorf("Unexpected issue for string: %s", issue.Str)
			continue
		}

		if issue.MatchingConst != expectedConst {
			t.Errorf("For string %q: got matching const %q, want %q",
				issue.Str, issue.MatchingConst, expectedConst)
		}
	}
}

func TestIssuePerFileThreeFiles(t *testing.T) {
	sources := map[string]string{
		"a.go": `package example
func a() { _ = "dup"; _ = "dup" }`,
		"b.go": `package example
func b() { _ = "dup" }`,
		"c.go": `package example
func c() { _ = "dup" }`,
	}

	fset := token.NewFileSet()
	var files []*ast.File
	for name, src := range sources {
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatalf("Failed to parse %s: %v", name, err)
		}
		files = append(files, f)
	}

	chkr, info := checker(fset)
	_ = chkr.Files(files)

	issues, err := Run(files, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 3 {
		t.Fatalf("len(issues) = %d, want 3", len(issues))
	}

	fileSet := make(map[string]bool)
	for _, issue := range issues {
		if issue.Str != "dup" {
			t.Errorf("Issue.Str = %v, want %v", issue.Str, "dup")
		}
		if issue.OccurrencesCount != 4 {
			t.Errorf("Issue.OccurrencesCount = %v, want 4", issue.OccurrencesCount)
		}
		fileSet[issue.Pos.Filename] = true
	}

	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if !fileSet[name] {
			t.Errorf("missing issue for file %s", name)
		}
	}
}

func TestIssuePerFile(t *testing.T) {
	code := `package example
func example() {
	a := "repeated"
	b := "repeated"
}`
	testCode := `package example
func testHelper() {
	c := "repeated"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse example.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "example_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse example_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Per-scope counting: "repeated" has 2 non-test occurrences and
	// 1 test occurrence. Only the non-test scope meets MinOccurrences=2.
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}

	issue := issues[0]
	if issue.Str != "repeated" {
		t.Errorf("Issue.Str = %v, want repeated", issue.Str)
	}
	if issue.OccurrencesCount != 2 {
		t.Errorf("Issue.OccurrencesCount = %v, want 2", issue.OccurrencesCount)
	}
	if issue.Pos.Filename != "example.go" {
		t.Errorf("Issue.Pos.Filename = %v, want example.go", issue.Pos.Filename)
	}
}

func TestRunWithConfigEmptyFiles(t *testing.T) {
	fset := token.NewFileSet()
	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{})

	issues, err := Run([]*ast.File{}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("len(issues) = %d, want 0", len(issues))
	}
}

func TestRunWithConfigIgnoreTests(t *testing.T) {
	code := `package example
func example() {
	a := "dup"
	b := "dup"
}`
	testCode := `package example
func testHelper() {
	c := "dup"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse example.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "example_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse example_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
		IgnoreTests:     true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	issue := issues[0]
	if issue.Pos.Filename != "example.go" {
		t.Errorf("Issue.Pos.Filename = %v, want example.go", issue.Pos.Filename)
	}
	if issue.OccurrencesCount != 2 {
		t.Errorf("Issue.OccurrencesCount = %v, want 2", issue.OccurrencesCount)
	}
}

func TestIssue57_TestFileInflation(t *testing.T) {
	code := `package example
func boolFunc() {
	_ = "false"
}`
	testCode := `package example
func testBoolFunc() {
	_ = "false"
	_ = "false"
	_ = "false"
	_ = "false"
	_ = "false"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "bool.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse bool.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "bool_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse bool_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  4,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// bool.go has 1 occurrence (below threshold of 4) — no issue.
	// bool_test.go has 5 occurrences (meets threshold) — issue emitted.
	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}

	issue := issues[0]
	if issue.Pos.Filename != "bool_test.go" {
		t.Errorf("Issue.Pos.Filename = %v, want bool_test.go", issue.Pos.Filename)
	}
	if issue.OccurrencesCount != 5 {
		t.Errorf("Issue.OccurrencesCount = %v, want 5", issue.OccurrencesCount)
	}
	if issue.Str != "false" {
		t.Errorf("Issue.Str = %v, want false", issue.Str)
	}
}

func TestIssue57_PerScopeThreshold(t *testing.T) {
	code := `package example
func prodFunc() {
	_ = "shared"
	_ = "shared"
}`
	testCode := `package example
func testFunc() {
	_ = "shared"
	_ = "shared"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "prod.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse prod.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "prod_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse prod_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  3,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Global count is 4 (survives ProcessResults with minOccurrences=3),
	// but nonTestCount=2 < 3 and testCount=2 < 3, so no issues emitted.
	if len(issues) != 0 {
		t.Errorf("len(issues) = %d, want 0", len(issues))
		for _, issue := range issues {
			t.Logf("Unexpected issue: %q at %s with %d occurrences",
				issue.Str, issue.Pos.Filename, issue.OccurrencesCount)
		}
	}
}

func TestIssue57_BothScopesMeetThreshold(t *testing.T) {
	code := `package example
func prodFunc() {
	_ = "common"
	_ = "common"
	_ = "common"
}`
	testCode := `package example
func testFunc() {
	_ = "common"
	_ = "common"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "lib.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse lib.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "lib_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse lib_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Both scopes meet threshold: 2 issues with per-scope counts.
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}

	counts := make(map[string]int)
	for _, issue := range issues {
		if issue.Str != "common" {
			t.Errorf("Issue.Str = %v, want common", issue.Str)
		}
		counts[issue.Pos.Filename] = issue.OccurrencesCount
	}

	if counts["lib.go"] != 3 {
		t.Errorf("lib.go OccurrencesCount = %v, want 3", counts["lib.go"])
	}
	if counts["lib_test.go"] != 2 {
		t.Errorf("lib_test.go OccurrencesCount = %v, want 2", counts["lib_test.go"])
	}
}

func TestMatchingConst_CrossScopeLeakage(t *testing.T) {
	// Reproduces cross-scope leakage: a constant declared only in a test
	// file should not be suggested as MatchingConst for a production issue,
	// because production code cannot reference test-only constants.
	code := `package example
func prodFunc() {
	_ = "magic-value"
	_ = "magic-value"
}`
	testCode := `package example
const TestMagic = "magic-value"
func testFunc() {
	_ = "magic-value"
	_ = "magic-value"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "prod.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse prod.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "prod_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse prod_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength:    3,
		MinOccurrences:     2,
		MatchWithConstants: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Both scopes meet threshold → 2 issues.
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}

	for _, issue := range issues {
		if issue.Pos.Filename == "prod.go" && issue.MatchingConst != "" {
			// BUG: prod.go issue references TestMagic which is defined
			// only in prod_test.go — production code cannot use it.
			t.Errorf("prod.go issue has MatchingConst=%q from test file; "+
				"production issues should not reference test-only constants",
				issue.MatchingConst)
		}
	}
}

func TestFindDuplicates_CrossScopeLeakage(t *testing.T) {
	// Reproduces cross-scope leakage for FindDuplicates: a constant
	// in a test file should not be reported as a duplicate of a
	// production constant (or vice versa), because they live in
	// different scopes.
	code := `package example
const ProdConst = "shared-value"
`
	testCode := `package example
const TestConst = "shared-value"
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "prod.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse prod.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "prod_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse prod_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  1,
		FindDuplicates:  true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, issue := range issues {
		if issue.DuplicateConst == "" {
			continue
		}
		issueIsTest := strings.HasSuffix(issue.Pos.Filename, "_test.go")
		dupIsTest := strings.HasSuffix(issue.DuplicatePos.Filename, "_test.go")
		if issueIsTest != dupIsTest {
			// BUG: cross-scope duplicate reported — a test constant
			// is flagged as duplicate of a production constant.
			t.Errorf("cross-scope duplicate: %s (%s) flagged as duplicate of %s (%s)",
				issue.Str, issue.Pos.Filename,
				issue.DuplicateConst, issue.DuplicatePos.Filename)
		}
	}
}

func TestMatchingConst_FindDuplicatesOnly(t *testing.T) {
	// When FindDuplicates is enabled but MatchWithConstants is not,
	// p.consts gets populated for duplicate detection. The reporting
	// loop should NOT set MatchingConst on string issues in this case.
	code := `package example
const MyConst = "some-value"
func example() {
	_ = "some-value"
	_ = "some-value"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, &Config{
		MinStringLength:    3,
		MinOccurrences:     2,
		FindDuplicates:     true,
		MatchWithConstants: false,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, issue := range issues {
		if issue.DuplicateConst != "" {
			continue // skip duplicate-const issues
		}
		if issue.MatchingConst != "" {
			// BUG: MatchingConst is set even though MatchWithConstants=false,
			// because FindDuplicates also populates p.consts.
			t.Errorf("MatchingConst = %q for string %q, want empty "+
				"(MatchWithConstants is false)", issue.MatchingConst, issue.Str)
		}
	}
}

func TestNondeterministicOutput(t *testing.T) {
	// With concurrent visitors, positions are appended in arbitrary
	// goroutine order. This can make issue positions and the selected
	// MatchingConst vary between runs. Verify output is deterministic.
	sources := map[string]string{
		"a.go": `package example
func a() { _ = "ndet"; _ = "ndet" }`,
		"b.go": `package example
func b() { _ = "ndet"; _ = "ndet" }`,
		"c.go": `package example
func c() { _ = "ndet"; _ = "ndet" }`,
		"d.go": `package example
func d() { _ = "ndet"; _ = "ndet" }`,
		"e.go": `package example
func e() { _ = "ndet"; _ = "ndet" }`,
	}

	// Run 10 times and collect the issue ordering each time.
	var firstRun []string
	for attempt := 0; attempt < 10; attempt++ {
		fset := token.NewFileSet()
		var files []*ast.File
		// Parse in deterministic order.
		for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go"} {
			f, err := parser.ParseFile(fset, name, sources[name], 0)
			if err != nil {
				t.Fatalf("Failed to parse %s: %v", name, err)
			}
			files = append(files, f)
		}

		chkr, info := checker(fset)
		_ = chkr.Files(files)

		issues, err := Run(files, fset, info, &Config{
			MinStringLength: 3,
			MinOccurrences:  2,
		})
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		var order []string
		for _, issue := range issues {
			order = append(order, fmt.Sprintf("%s:%d", issue.Pos.Filename, issue.Pos.Line))
		}

		if attempt == 0 {
			firstRun = order
		} else {
			if len(order) != len(firstRun) {
				t.Fatalf("attempt %d: got %d issues, first run had %d", attempt, len(order), len(firstRun))
			}
			for i := range order {
				if order[i] != firstRun[i] {
					// BUG: issue ordering varies between runs due to
					// nondeterministic concurrent visitor appends.
					t.Errorf("attempt %d: issue[%d] = %s, first run had %s",
						attempt, i, order[i], firstRun[i])
				}
			}
		}
	}
}

func TestMatchWithConstantsAndFindDuplicates(t *testing.T) {
	// Both MatchWithConstants and FindDuplicates enabled — a realistic
	// golangci-lint config. Verify matching constant resolution and
	// duplicate detection work correctly together.
	code := `package example
const ProdConst = "shared"
func example() {
	_ = "shared"
	_ = "shared"
}`
	code2 := `package example
const ProdConst2 = "shared"
`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "a.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse a.go: %v", err)
	}
	f2, err := parser.ParseFile(fset, "b.go", code2, 0)
	if err != nil {
		t.Fatalf("Failed to parse b.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, f2})

	issues, err := Run([]*ast.File{f, f2}, fset, info, &Config{
		MinStringLength:    3,
		MinOccurrences:     2,
		MatchWithConstants: true,
		FindDuplicates:     true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	var stringIssues, dupIssues []Issue
	for _, issue := range issues {
		if issue.DuplicateConst != "" {
			dupIssues = append(dupIssues, issue)
		} else {
			stringIssues = append(stringIssues, issue)
		}
	}

	// String issues should have MatchingConst set.
	for _, issue := range stringIssues {
		if issue.Str != "shared" {
			continue
		}
		if issue.MatchingConst == "" {
			t.Errorf("string issue at %s missing MatchingConst", issue.Pos.Filename)
		}
	}

	// Duplicate const issue: ProdConst2 is a duplicate of ProdConst
	// (or vice versa, depending on position sort order).
	if len(dupIssues) != 1 {
		t.Fatalf("len(dupIssues) = %d, want 1", len(dupIssues))
	}
	if dupIssues[0].DuplicateConst == "" {
		t.Error("duplicate issue missing DuplicateConst")
	}
}

func TestTestOnlyString(t *testing.T) {
	// String appears only in test files with IgnoreTests=false.
	// Should produce a test-file issue but no production issue.
	code := `package example
func prodFunc() {
	_ = "other"
}`
	testCode := `package example
func testFunc() {
	_ = "testonly"
	_ = "testonly"
	_ = "testonly"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "prod.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse prod.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "prod_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse prod_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, issue := range issues {
		if issue.Str != "testonly" {
			continue
		}
		if issue.Pos.Filename != "prod_test.go" {
			t.Errorf("testonly issue at %s, want prod_test.go", issue.Pos.Filename)
		}
		if issue.OccurrencesCount != 3 {
			t.Errorf("testonly OccurrencesCount = %d, want 3", issue.OccurrencesCount)
		}
	}

	// Verify no prod.go issue for "testonly" (it doesn't appear there).
	for _, issue := range issues {
		if issue.Str == "testonly" && issue.Pos.Filename == "prod.go" {
			t.Error("unexpected prod.go issue for test-only string")
		}
	}
}

func TestMatchingConst_CrossScopePreference(t *testing.T) {
	// When constants exist in both scopes, non-test issues should use
	// the non-test constant. Test issues should prefer the non-test
	// constant but fall back to test constants.
	code := `package example
const ProdConst = "val"
func prodFunc() {
	_ = "val"
	_ = "val"
}`
	testCode := `package example
const TestConst = "val"
func testFunc() {
	_ = "val"
	_ = "val"
}`

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "lib.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse lib.go: %v", err)
	}
	fTest, err := parser.ParseFile(fset, "lib_test.go", testCode, 0)
	if err != nil {
		t.Fatalf("Failed to parse lib_test.go: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f, fTest})

	issues, err := Run([]*ast.File{f, fTest}, fset, info, &Config{
		MinStringLength:    3,
		MinOccurrences:     2,
		MatchWithConstants: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, issue := range issues {
		if issue.Str != "val" {
			continue
		}
		switch issue.Pos.Filename {
		case "lib.go":
			if issue.MatchingConst != "ProdConst" {
				t.Errorf("lib.go MatchingConst = %q, want ProdConst",
					issue.MatchingConst)
			}
		case "lib_test.go":
			// Test issue should prefer non-test constant (ProdConst)
			// since test code can reference production constants.
			if issue.MatchingConst != "ProdConst" {
				t.Errorf("lib_test.go MatchingConst = %q, want ProdConst",
					issue.MatchingConst)
			}
		}
	}
}

func TestNewWithIgnorePatterns_InvalidRegex(t *testing.T) {
	code := `package example
func example() {
	a := "repeated"
	b := "repeated"
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	// Invalid regex pattern should not panic — graceful degradation
	issues, err := Run([]*ast.File{f}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
		IgnoreStrings:   []string{"[invalid"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	// The invalid regex is silently skipped, so "repeated" should still be found
	if len(issues) != 1 {
		t.Errorf("len(issues) = %d, want 1", len(issues))
	}
}

func TestRunWithConfig_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	// Create 5 files each with the shared string "concurrent" appearing twice
	var files []*ast.File
	fset := token.NewFileSet()
	for i := 0; i < 5; i++ {
		code := `package example
func f() {
	a := "concurrent"
	b := "concurrent"
}`
		f, err := parser.ParseFile(fset, fmt.Sprintf("file%d.go", i), code, 0)
		if err != nil {
			t.Fatalf("Failed to parse file%d.go: %v", i, err)
		}
		files = append(files, f)
	}

	chkr, info := checker(fset)
	_ = chkr.Files(files)

	issues, err := Run(files, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should have 5 issues (one per file)
	if len(issues) != 5 {
		t.Fatalf("len(issues) = %d, want 5", len(issues))
	}

	for _, issue := range issues {
		if issue.OccurrencesCount != 10 {
			t.Errorf("Issue.OccurrencesCount = %v, want 10", issue.OccurrencesCount)
		}
	}
}

func TestRunWithConfig_DuplicateConsts(t *testing.T) {
	code := `package example
const ConstA = "shared_value"
const ConstB = "shared_value"
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, &Config{
		FindDuplicates: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}

	issue := issues[0]
	if issue.DuplicateConst == "" {
		t.Error("Issue.DuplicateConst is empty, want a constant name")
	}
	if issue.DuplicatePos.Filename == "" {
		t.Error("Issue.DuplicatePos.Filename is empty")
	}
	if issue.Str != "shared_value" {
		t.Errorf("Issue.Str = %v, want shared_value", issue.Str)
	}
}

func TestNewWithIgnorePatterns_MultiplePatterns(t *testing.T) {
	code := `package example
func example() {
	a := "test_val"
	b := "test_val"
	c := "foo_val"
	d := "foo_val"
	e := "keep_val"
	f := "keep_val"
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
		IgnoreStrings:   []string{"test", "foo"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if issues[0].Str != "keep_val" {
		t.Errorf("Issue.Str = %v, want keep_val", issues[0].Str)
	}
}

func TestRunWithConfig_IgnoreFunctions(t *testing.T) {
	code := `package example
import "log/slog"
func example() {
	slog.Info("msg")
	slog.Info("msg")
	println("other")
	println("other")
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
		IgnoreFunctions: []string{"slog.Info"},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if issues[0].Str != "other" {
		t.Errorf("Issue.Str = %v, want other", issues[0].Str)
	}
}

func TestRunWithConfig_IgnoreFunctions_Empty(t *testing.T) {
	code := `package example
func example() {
	println("msg")
	println("msg")
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "example.go", code, 0)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	issues, err := Run([]*ast.File{f}, fset, info, &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
		IgnoreFunctions: nil,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("len(issues) = %d, want 1", len(issues))
	}
	if issues[0].Str != "msg" {
		t.Errorf("Issue.Str = %v, want msg", issues[0].Str)
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
