package goconst

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestMatchConstant(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		wantIssues  int
		wantMatches map[string]string // string -> matching const name
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

			chkr, info := checker(fset)
			_ = chkr.Files([]*ast.File{f})

			issues, err := Run([]*ast.File{f}, fset, info, config)
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

	chkr, info := checker(fset)
	_ = chkr.Files(astFiles)

	issues, err := Run(astFiles, fset, info, config)
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

func TestMatchConstantExpressions(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		evalExpr    bool
		wantIssues  int
		wantMatches map[string]string // string -> matching const name
	}{
		{
			name: "simple string concatenation",
			code: `package example
const (
	Prefix = "api."
	Endpoint = Prefix + "users"
)
func example() {
	path := "api.users"
}`,
			evalExpr:   true,
			wantIssues: 1,
			wantMatches: map[string]string{
				"api.users": "Endpoint",
			},
		},
		{
			name: "nested expressions",
			code: `package example
const (
	BaseURL = "example.com"
	APIPath = "/api/v1"
	FullURL = BaseURL + APIPath
)
func example() {
	url := "example.com/api/v1"
}`,
			evalExpr:   true,
			wantIssues: 1,
			wantMatches: map[string]string{
				"example.com/api/v1": "FullURL",
			},
		},
		{
			name: "expressions with special characters",
			code: `package example
const (
	ErrorPrefix = "ERROR: "
	ErrorMsg = ErrorPrefix + "invalid\ninput"
)
func example() {
	msg := "ERROR: invalid\ninput"
}`,
			evalExpr:   true,
			wantIssues: 1,
			wantMatches: map[string]string{
				"ERROR: invalid\ninput": "ErrorMsg",
			},
		},
		{
			name: "multiple levels of indirection",
			code: `package example
const (
	A = "a"
	B = A + "b"
	C = B + "c"
	D = C + "d"
)
func example() {
	val := "abcd"
}`,
			evalExpr:   true,
			wantIssues: 1,
			wantMatches: map[string]string{
				"abcd": "D",
			},
		},
		{
			name: "constant expression - feature disabled",
			code: `package example
const (
	Prefix = "api."
	Endpoint = Prefix + "users"
)
func example() {
	path := "api.users"
}`,
			evalExpr:   false,
			wantIssues: 1, // Still detects the string, but no matching constant
			wantMatches: map[string]string{
				"api.users": "", // Empty string indicates no constant match
			},
		},
		{
			name: "parenthesized expressions",
			code: `package example
const (
	A = "a"
	B = "b"
	Combined = (A + B) + "c"
)
func example() {
	val := "abc"
}`,
			evalExpr:   true,
			wantIssues: 1,
			wantMatches: map[string]string{
				"abc": "Combined",
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
				MinStringLength:      1, // Set to 1 to catch all strings
				MinOccurrences:       1, // Set to 1 to catch all occurrences
				MatchWithConstants:   true,
				EvalConstExpressions: tt.evalExpr,
			}

			chkr, info := checker(fset)
			_ = chkr.Files([]*ast.File{f})

			issues, err := Run([]*ast.File{f}, fset, info, config)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			if len(issues) != tt.wantIssues {
				t.Errorf("Got %d issues, want %d", len(issues), tt.wantIssues)
				for _, issue := range issues {
					t.Logf("Found issue: %q matches constant %q", issue.Str, issue.MatchingConst)
				}
				return
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
