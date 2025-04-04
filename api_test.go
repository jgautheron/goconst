package goconst

import (
	"go/ast"
	"go/parser"
	"go/token"
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
				IgnoreStrings:   "test",
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

			issues, err := Run([]*ast.File{f}, fset, tt.config)
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

	issues, err := Run([]*ast.File{f}, fset, config)
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
	code1 := `package example
func example1() {
	a := "shared"
	b := "shared"
}
`
	code2 := `package example
func example2() {
	c := "shared"
	d := "unique"
}
`
	fset := token.NewFileSet()
	f1, err := parser.ParseFile(fset, "file1.go", code1, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	f2, err := parser.ParseFile(fset, "file2.go", code2, 0)
	if err != nil {
		t.Fatalf("Failed to parse test code: %v", err)
	}

	config := &Config{
		MinStringLength: 3,
		MinOccurrences:  2,
	}

	issues, err := Run([]*ast.File{f1, f2}, fset, config)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Should find "shared" appearing 3 times across both files
	if len(issues) != 1 {
		t.Fatalf("Expected 1 issue, got %d", len(issues))
	}

	issue := issues[0]
	if issue.Str != "shared" {
		t.Errorf("Issue.Str = %v, want %v", issue.Str, "shared")
	}
	if issue.OccurrencesCount != 3 {
		t.Errorf("Issue.OccurrencesCount = %v, want 3", issue.OccurrencesCount)
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

	issues, err := Run([]*ast.File{f}, fset, config)
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

			issues, err := Run([]*ast.File{f}, fset, config)
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
