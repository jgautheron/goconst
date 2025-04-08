package goconst

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"testing"
)

func TestTreeVisitor_Visit(t *testing.T) {
	tests := []struct {
		name                string
		code                string
		expectedStrings     []string
		expectedConstCounts map[string]int
		excludeTypes        map[Type]bool
	}{
		{
			name: "assignment detection",
			code: `package example
func example() {
	a := "test"
}`,
			expectedStrings:     []string{"test"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "binary expression detection",
			code: `package example
func example() {
	if a == "test" {}
}`,
			expectedStrings:     []string{"test"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "case clause detection",
			code: `package example
func example() {
	switch a {
	case "test":
	}
}`,
			expectedStrings:     []string{"test"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "return statement detection",
			code: `package example
func example() string {
	return "test"
}`,
			expectedStrings:     []string{"test"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "function call detection",
			code: `package example
func example() {
	println("test")
}`,
			expectedStrings:     []string{"test"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "excluded type assignment",
			code: `package example
func example() {
	a := "test"
}`,
			expectedStrings:     []string{},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{Assignment: true},
		},
		{
			name: "constant detection",
			code: `package example
const MyConst = "test"
func example() {
}`,
			expectedStrings:     []string{},
			expectedConstCounts: map[string]int{"test": 1},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "detect multiple constants",
			code: `package example
const MyConst1 = "test"
const MyConst2 = "test"
func example() {
	const inFunc = "test"
}`,
			expectedStrings:     []string{},
			expectedConstCounts: map[string]int{"test": 3},
			excludeTypes:        map[Type]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "example.go", tt.code, 0)
			if err != nil {
				t.Fatalf("Failed to parse test code: %v", err)
			}

			p := &Parser{
				minLength:        3,
				minOccurrences:   1,
				supportedTokens:  []token.Token{token.STRING},
				excludeTypes:     tt.excludeTypes,
				strs:             Strings{},
				consts:           Constants{},
				matchConstant:    true,
				findDuplicates:   true,
				stringCount:      make(map[string]int),
				stringMutex:      sync.RWMutex{},
				stringCountMutex: sync.RWMutex{},
			}

			v := &treeVisitor{
				p:           p,
				fileSet:     fset,
				packageName: "example",
			}

			ast.Walk(v, f)

			// Check that we found the expected strings
			foundStrs := make(map[string]bool)
			for str := range p.strs {
				foundStrs[str] = true
			}

			for _, expectedStr := range tt.expectedStrings {
				if !foundStrs[expectedStr] {
					t.Errorf("Expected string %q not found in results", expectedStr)
				}
			}

			// Check that we didn't find any unexpected strings
			if len(foundStrs) != len(tt.expectedStrings) {
				t.Errorf("Found %d strings, expected %d", len(foundStrs), len(tt.expectedStrings))
			}

			// Check that we found the expected constants
			foundConstCounts := make(map[string]int)
			for val, consts := range p.consts {
				foundConstCounts[val] = len(consts)
			}

			for expectedConst, expectedCount := range tt.expectedConstCounts {
				if foundConstCounts[expectedConst] != expectedCount {
					t.Errorf("Expected %d occurrences of const %q, found %d", expectedCount, expectedConst,
						foundConstCounts[expectedConst])
				}
			}

			if len(foundConstCounts) != len(tt.expectedConstCounts) {
				t.Errorf("Found %d const values, expected %d", len(foundConstCounts), len(tt.expectedConstCounts))
			}
		})
	}
}

func TestTreeVisitor_AddString(t *testing.T) {
	tests := []struct {
		name         string
		str          string
		typ          Type
		excludeTypes map[Type]bool
		minLength    int
		expectAdded  bool
	}{
		{
			name:         "basic string",
			str:          `"test"`,
			typ:          Assignment,
			excludeTypes: map[Type]bool{},
			minLength:    3,
			expectAdded:  true,
		},
		{
			name:         "excluded type",
			str:          `"test"`,
			typ:          Assignment,
			excludeTypes: map[Type]bool{Assignment: true},
			minLength:    3,
			expectAdded:  false,
		},
		{
			name:         "too short",
			str:          `"ab"`,
			typ:          Assignment,
			excludeTypes: map[Type]bool{},
			minLength:    3,
			expectAdded:  false,
		},
		{
			name:         "raw string literal",
			str:          "`test`",
			typ:          Assignment,
			excludeTypes: map[Type]bool{},
			minLength:    3,
			expectAdded:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Parser{
				minLength:        tt.minLength,
				excludeTypes:     tt.excludeTypes,
				strs:             Strings{},
				stringCount:      make(map[string]int),
				stringMutex:      sync.RWMutex{},
				stringCountMutex: sync.RWMutex{},
			}

			fset := token.NewFileSet()
			v := &treeVisitor{
				p:           p,
				fileSet:     fset,
				packageName: "example",
			}

			v.addString(tt.str, token.Pos(1), tt.typ)

			// Check if the string was added
			if tt.expectAdded {
				if len(p.strs) != 1 {
					t.Errorf("Expected string to be added, but it wasn't")
				}
			} else {
				if len(p.strs) != 0 {
					t.Errorf("Expected string not to be added, but it was")
				}
			}
		})
	}
}
