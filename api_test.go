package goconst

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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

	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}

	files := make(map[string]bool)
	for _, issue := range issues {
		if issue.Str != "repeated" {
			t.Errorf("Issue.Str = %v, want %v", issue.Str, "repeated")
		}
		if issue.OccurrencesCount != 3 {
			t.Errorf("Issue.OccurrencesCount = %v, want 3", issue.OccurrencesCount)
		}
		files[issue.Pos.Filename] = true
	}

	if !files["example.go"] {
		t.Errorf("Issue.Pos.Filename: missing example.go")
	}
	if !files["example_test.go"] {
		t.Errorf("Issue.Pos.Filename: missing example_test.go")
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

func checker(fset *token.FileSet) (*types.Checker, *types.Info) {
	cfg := &types.Config{
		Error: func(err error) {},
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue),
	}
	return types.NewChecker(cfg, fset, types.NewPackage("", "example"), info), info
}
