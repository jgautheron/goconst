package goconst

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestMatchConstant(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		wantIssues    int
		wantMatches   map[string]string // string -> matching const name
	}{
		{
			name: "basic constant match",
			code: `package example
const MyConst = "test"
func example() {
	str := "test"
}`,
			wantIssues: 1,
			wantMatches: map[string]string{
				"test": "MyConst",
			},
		},
		{
			name: "multiple constants same value",
			code: `package example
const (
	FirstConst = "test"
	SecondConst = "test"
)
func example() {
	str := "test"
}`,
			wantIssues: 1,
			wantMatches: map[string]string{
				"test": "FirstConst", // Should match the first one defined
			},
		},
		{
			name: "constant after usage",
			code: `package example
func example() {
	str := "test"
}
const MyConst = "test"`,
			wantIssues: 1,
			wantMatches: map[string]string{
				"test": "MyConst",
			},
		},
		{
			name: "constant in init",
			code: `package example
func init() {
	const InitConst = "test"
}
func example() {
	str := "test"
}`,
			wantIssues: 1,
			wantMatches: map[string]string{
				"test": "InitConst",
			},
		},
		{
			name: "constant in different scope",
			code: `package example
const GlobalConst = "global"
func example() {
	const localConst = "local"
	str1 := "global"
	str2 := "local"
}`,
			wantIssues: 2,
			wantMatches: map[string]string{
				"global": "GlobalConst",
				"local":  "localConst",
			},
		},
		{
			name: "exported vs unexported constants",
			code: `package example
const (
	ExportedConst = "exported"
	unexportedConst = "unexported"
)
func example() {
	str1 := "exported"
	str2 := "unexported"
}`,
			wantIssues: 2,
			wantMatches: map[string]string{
				"exported":   "ExportedConst",
				"unexported": "unexportedConst",
			},
		},
		{
			name: "string matches multiple constants",
			code: `package example
const (
	Const1 = "duplicate"
	Const2 = "duplicate"
	Const3 = "duplicate"
)
func example() {
	str := "duplicate"
}`,
			wantIssues: 1,
			wantMatches: map[string]string{
				"duplicate": "Const1", // Should match the first one defined
			},
		},
		{
			name: "constant with special characters",
			code: `package example
const SpecialConst = "test\nwith\tspecial\rchars"
func example() {
	str := "test\nwith\tspecial\rchars"
}`,
			wantIssues: 1,
			wantMatches: map[string]string{
				"test\nwith\tspecial\rchars": "SpecialConst",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "example.go", tt.code, 0)
			if err != nil {
				t.Fatalf("Failed to parse test code: %v", err)
			}

			config := &Config{
				MinStringLength:    1, // Set to 1 to catch all strings
				MinOccurrences:     1, // Set to 1 to catch all occurrences
				MatchWithConstants: true,
			}

			issues, err := Run([]*ast.File{f}, fset, config)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if len(issues) != tt.wantIssues {
				t.Errorf("Got %d issues, want %d", len(issues), tt.wantIssues)
				for _, issue := range issues {
					t.Logf("Found issue: %q matches constant %q", issue.Str, issue.MatchingConst)
				}
			}

			// Verify constant matches
			for _, issue := range issues {
				if wantConst, ok := tt.wantMatches[issue.Str]; ok {
					if issue.MatchingConst != wantConst {
						t.Errorf("String %q matched with constant %q, want %q", 
							issue.Str, issue.MatchingConst, wantConst)
					}
				} else {
					t.Errorf("Unexpected string found: %q", issue.Str)
				}
			}
		})
	}
}

func TestMatchConstantMultiFile(t *testing.T) {
	// Test constant matching across multiple files
	files := map[string]string{
		"const.go": `package example
const (
	SharedConst = "shared"
	PackageConst = "package"
)`,
		"main.go": `package example
func main() {
	str1 := "shared"
	str2 := "package"
}`,
	}

	fset := token.NewFileSet()
	astFiles := make([]*ast.File, 0, len(files))

	for name, content := range files {
		f, err := parser.ParseFile(fset, name, content, 0)
		if err != nil {
			t.Fatalf("Failed to parse %s: %v", name, err)
		}
		astFiles = append(astFiles, f)
	}

	config := &Config{
		MinStringLength:    1,
		MinOccurrences:     1,
		MatchWithConstants: true,
	}

	issues, err := Run(astFiles, fset, config)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	wantMatches := map[string]string{
		"shared":  "SharedConst",
		"package": "PackageConst",
	}

	if len(issues) != len(wantMatches) {
		t.Errorf("Got %d issues, want %d", len(issues), len(wantMatches))
	}

	for _, issue := range issues {
		if wantConst, ok := wantMatches[issue.Str]; ok {
			if issue.MatchingConst != wantConst {
				t.Errorf("String %q matched with constant %q, want %q",
					issue.Str, issue.MatchingConst, wantConst)
			}
		} else {
			t.Errorf("Unexpected string found: %q", issue.Str)
		}
	}
} 