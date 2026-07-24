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
		ignoreMapKeys       bool
		supportedTokens     []token.Token // defaults to STRING only when nil
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
			name: "composite literal detection",
			code: `package example
type person struct {
	name string
}

func example() {
	_ = []string{"test"}
	_ = map[string]string{"test": "value"}
	_ = person{name: "test"}
}`,
			expectedStrings:     []string{"test", "value"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "composite literal with non-literal elements",
			code: `package example
func example() {
	_ = [][]string{{"nested"}}
	_ = []string{getString()}
}
func getString() string { return "" }`,
			expectedStrings:     []string{"nested"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "excluded composite literal",
			code: `package example
func example() {
	_ = []string{"test"}
}`,
			expectedStrings:     []string{},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{CompositeLit: true},
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
		{
			name: "non-equality binary operators ignored",
			code: `package example
func example() {
	var a, b string
	if a < "foo" {}
	if b > "bar" {}
}`,
			expectedStrings:     []string{},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "map keys ignored, values kept",
			code: `package example
func example() {
	_ = map[string]string{"key": "value1"}
	_ = map[string]string{"key": "value2"}
}`,
			expectedStrings:     []string{"value1", "value2"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
		},
		{
			name: "map keys kept when not ignored",
			code: `package example
func example() {
	_ = map[string]string{"key": "value1"}
}`,
			expectedStrings:     []string{"key", "value1"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			name: "ignore-map-keys leaves slice and struct literals untouched",
			code: `package example
type person struct {
	name string
}

func example() {
	_ = []string{"aaa"}
	_ = person{name: "bbb"}
}`,
			expectedStrings:     []string{"aaa", "bbb"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
		},
		{
			// Named map type: the literal's AST type is an *ast.Ident, so this
			// relies on string keys being recognized without type information.
			name: "map keys ignored for named map type without type info",
			code: `package example
type M map[string]string
func example() {
	_ = M{"key": "value1"}
	_ = M{"key": "value2"}
}`,
			expectedStrings:     []string{"value1", "value2"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
		},
		{
			// Nested elided map literals have a nil AST type; string keys must
			// still be ignored without type information.
			name: "map keys ignored for elided nested map literals",
			code: `package example
func example() {
	_ = []map[string]string{
		{"key": "value1"},
		{"key": "value2"},
	}
}`,
			expectedStrings:     []string{"value1", "value2"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
		},
		{
			// The skip targets the KeyValueExpr key node, not the string value:
			// the same literal used elsewhere (here as an assignment) is kept.
			name: "map key skip is per-occurrence, not a global blacklist",
			code: `package example
func example() {
	_ = map[string]string{"dup": "value"}
	a := "dup"
	_ = a
}`,
			expectedStrings:     []string{"dup", "value"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
		},
		{
			// IgnoreMapKeys is scoped to string keys by design; numeric keys are
			// left in place (they are indistinguishable from array indices
			// without type information).
			name: "numeric map keys still reported (string-only scope)",
			code: `package example
func example() {
	_ = map[int]string{100: "value"}
}`,
			expectedStrings:     []string{"100", "value"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
			supportedTokens:     []token.Token{token.STRING, token.INT},
		},
		{
			// A raw (backtick) string key is also token.STRING and must be ignored.
			name: "raw string map keys are ignored too",
			code: "package example\n" +
				"func example() {\n" +
				"\t_ = map[string]string{`rawkey`: \"value1\"}\n" +
				"\t_ = map[string]string{`rawkey`: \"value2\"}\n" +
				"}",
			expectedStrings:     []string{"value1", "value2"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
		},
		{
			// Key wrapped in a builtin conversion: the string subtree must be
			// pruned so the later CallExpr visit does not record it.
			name: "converted string map keys are ignored",
			code: `package example
func example() {
	_ = map[string]string{string("key"): "value"}
}`,
			expectedStrings:     []string{"value"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
		},
		{
			// Named-string-typed key via conversion (map[K]V{K("role"): ...}).
			name: "converted named-string map keys are ignored",
			code: `package example
type K string
func example() {
	_ = map[K]string{K("role"): "value"}
}`,
			expectedStrings:     []string{"value"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
		},
		{
			name: "converted map keys kept when not ignored",
			code: `package example
type K string
func example() {
	_ = map[K]string{K("role"): "value"}
}`,
			expectedStrings:     []string{"role", "value"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
		},
		{
			// Array index expressions are not map keys; their literals must
			// survive even with ignore-map-keys enabled (isMap gate).
			name: "numeric array index expressions are not suppressed",
			code: `package example
func example() {
	_ = [500]string{int(100): "value"}
}`,
			expectedStrings:     []string{"100", "value"},
			expectedConstCounts: map[string]int{},
			excludeTypes:        map[Type]bool{},
			ignoreMapKeys:       true,
			supportedTokens:     []token.Token{token.STRING, token.INT},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "example.go", tt.code, 0)
			if err != nil {
				t.Fatalf("Failed to parse test code: %v", err)
			}

			supportedTokens := tt.supportedTokens
			if supportedTokens == nil {
				supportedTokens = []token.Token{token.STRING}
			}

			p := &Parser{
				minLength:        3,
				minOccurrences:   1,
				supportedTokens:  supportedTokens,
				excludeTypes:     tt.excludeTypes,
				ignoreMapKeys:    tt.ignoreMapKeys,
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

func TestTreeVisitor_AllPositionsRecorded(t *testing.T) {
	tests := []struct {
		name             string
		code             string
		minOccurrences   int
		str              string
		expectedPosCount int
	}{
		{
			name: "five occurrences with min 2",
			code: `package example
import "fmt"
func example() {
	a := "hello"
	if a == "hello" {
		fmt.Println("hello")
	}
	switch a {
	case "hello":
	}
	b := "hello"
	fmt.Println(b)
}`,
			minOccurrences:   2,
			str:              "hello",
			expectedPosCount: 5,
		},
		{
			name: "five occurrences with min 3",
			code: `package example
import "fmt"
func example() {
	a := "hello"
	if a == "hello" {
		fmt.Println("hello")
	}
	switch a {
	case "hello":
	}
	b := "hello"
	fmt.Println(b)
}`,
			minOccurrences:   3,
			str:              "hello",
			expectedPosCount: 5,
		},
		{
			name: "five occurrences with min 5",
			code: `package example
import "fmt"
func example() {
	a := "hello"
	if a == "hello" {
		fmt.Println("hello")
	}
	switch a {
	case "hello":
	}
	b := "hello"
	fmt.Println(b)
}`,
			minOccurrences:   5,
			str:              "hello",
			expectedPosCount: 5,
		},
		{
			name: "three occurrences with min 2",
			code: `package example
func example() string {
	a := "world"
	b := "world"
	return "world"
}`,
			minOccurrences:   2,
			str:              "world",
			expectedPosCount: 3,
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
				minOccurrences:   tt.minOccurrences,
				supportedTokens:  []token.Token{token.STRING},
				excludeTypes:     map[Type]bool{},
				strs:             Strings{},
				consts:           Constants{},
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

			positions, ok := p.strs[tt.str]
			if !ok {
				t.Fatalf("Expected string %q in results, but it was not found", tt.str)
			}

			if len(positions) != tt.expectedPosCount {
				t.Errorf("Expected %d positions for %q, got %d",
					tt.expectedPosCount, tt.str, len(positions))
				for i, pos := range positions {
					t.Logf("  position %d: %s", i, pos.String())
				}
			}

			count := p.GetStringCount(tt.str)
			if count != tt.expectedPosCount {
				t.Errorf("Expected string count %d for %q, got %d",
					tt.expectedPosCount, tt.str, count)
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
			name:         "too short more than one byte for char",
			str:          `"да"`,
			typ:          Assignment,
			excludeTypes: map[Type]bool{},
			minLength:    3,
			expectAdded:  false,
		},
		{
			name:         "unicode string counted by runes",
			str:          `"привет"`,
			typ:          Assignment,
			excludeTypes: map[Type]bool{},
			minLength:    3,
			expectAdded:  true,
		},
		{
			name:         "unicode string exactly at threshold",
			str:          `"猫犬鳥"`,
			typ:          Assignment,
			excludeTypes: map[Type]bool{},
			minLength:    3,
			expectAdded:  true,
		},
		{
			name:         "raw string literal",
			str:          "`test`",
			typ:          Assignment,
			excludeTypes: map[Type]bool{},
			minLength:    3,
			expectAdded:  true,
		},
		{
			name:         "malformed quote fallback",
			str:          `"unclosed`, // strconv.Unquote fails; fallback strips first+last byte → "unclose"
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

func TestTreeVisitor_AddConst_UnicodeMinLength(t *testing.T) {
	fset := token.NewFileSet()

	p := &Parser{
		minLength:        3,
		matchConstant:    true,
		findDuplicates:   true,
		supportedTokens:  []token.Token{token.STRING},
		strs:             Strings{},
		consts:           Constants{},
		stringCount:      make(map[string]int),
		stringMutex:      sync.RWMutex{},
		stringCountMutex: sync.RWMutex{},
	}

	v := &treeVisitor{
		p:           p,
		fileSet:     fset,
		packageName: "example",
	}

	v.addConst("TooShort", `"да"`, token.Pos(1))
	v.addConst("LongEnough", `"привет"`, token.Pos(2))

	if _, ok := p.consts["да"]; ok {
		t.Error("did not expect short unicode const to be added")
	}
	if _, ok := p.consts["привет"]; !ok {
		t.Error("expected unicode const to be added")
	}
}

func TestTreeVisitor_AddString_NumberRange(t *testing.T) {
	tests := []struct {
		name        string
		str         string
		numberMin   int
		numberMax   int
		expectAdded bool
	}{
		{name: "below min", str: "50", numberMin: 100, numberMax: 200, expectAdded: false},
		{name: "in range", str: "150", numberMin: 100, numberMax: 200, expectAdded: true},
		{name: "above max", str: "250", numberMin: 100, numberMax: 200, expectAdded: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Parser{
				minLength:        1,
				numberMin:        tt.numberMin,
				numberMax:        tt.numberMax,
				excludeTypes:     map[Type]bool{},
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

			v.addString(tt.str, token.Pos(1), Assignment)

			if tt.expectAdded {
				if len(p.strs) != 1 {
					t.Errorf("expected string to be added, but it wasn't")
				}
			} else {
				if len(p.strs) != 0 {
					t.Errorf("expected string not to be added, but it was")
				}
			}
		})
	}
}

func TestTreeVisitor_ShouldIgnoreCall(t *testing.T) {
	tests := []struct {
		name            string
		code            string
		ignoreFunctions map[string]struct{}
		expectStrings   int
	}{
		{
			name: "slog.Info ignored",
			code: `package example
import "log/slog"
func example() {
	slog.Info("msg")
	slog.Info("msg")
}`,
			ignoreFunctions: map[string]struct{}{"slog.Info": {}},
			expectStrings:   0,
		},
		{
			name: "println not ignored when slog.Info is",
			code: `package example
func example() {
	println("msg")
	println("msg")
}`,
			ignoreFunctions: map[string]struct{}{"slog.Info": {}},
			expectStrings:   1,
		},
		{
			name: "empty ignore list ignores nothing",
			code: `package example
import "log/slog"
func example() {
	slog.Info("msg")
	slog.Info("msg")
}`,
			ignoreFunctions: nil,
			expectStrings:   1,
		},
		{
			name: "direct function call ignored",
			code: `package example
func example() {
	println("msg")
	println("msg")
}`,
			ignoreFunctions: map[string]struct{}{"println": {}},
			expectStrings:   0,
		},
		{
			name: "multiple ignored functions",
			code: `package example
import "fmt"
func example() {
	fmt.Println("keep")
	fmt.Println("keep")
	fmt.Errorf("skip")
	fmt.Errorf("skip")
}`,
			ignoreFunctions: map[string]struct{}{"fmt.Errorf": {}},
			expectStrings:   1, // only "keep" via fmt.Println
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
				minOccurrences:   2,
				supportedTokens:  []token.Token{token.STRING},
				excludeTypes:     map[Type]bool{},
				ignoreFunctions:  tt.ignoreFunctions,
				strs:             Strings{},
				consts:           Constants{},
				matchConstant:    false,
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
			p.ProcessResults()

			if len(p.strs) != tt.expectStrings {
				t.Errorf("expected %d strings, got %d", tt.expectStrings, len(p.strs))
				for str, positions := range p.strs {
					t.Logf("  found: %q (%d occurrences)", str, len(positions))
				}
			}
		})
	}
}
