package goconst

import (
	"testing"
)

func TestIntegrationWithTestdata(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		ignoreTests     bool
		matchConstant   bool
		numbers         bool
		numberMin       int
		numberMax       int
		minLength       int
		findDuplicates  bool
		evalConstExpr   bool
		minOccurrences  int
		expectedStrings int
		expectedMatches map[string]string // string -> expected matching constant
	}{
		{
			name:            "basic duplicate string detection",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         false,
			findDuplicates:  false,
			evalConstExpr:   false,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 9, // All strings that appear at least twice (7 original + 2 from const_expressions.go)
		},
		{
			name:            "match with constants",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   true,
			numbers:         false,
			findDuplicates:  false,
			evalConstExpr:   true, // Enable constant expression evaluation for this test
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 9, // All strings that appear at least twice (7 original + 2 from const_expressions.go)
			expectedMatches: map[string]string{
				"single constant":             "SingleConst",
				"grouped constant":            "GroupedConst1",
				"duplicate value":             "DuplicateConst1",
				"special\nvalue\twith\rchars": "SpecialConst",
				"example.com/api":             "API", // from const_expressions.go
				"example.com/web":             "Web", // from const_expressions.go
			},
		},
		{
			name:            "include numbers",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         true,
			findDuplicates:  false,
			evalConstExpr:   false,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 10, // All strings + "12345" (8 original + 2 from const_expressions.go)
		},
		{
			name:            "filter by number range",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         true,
			numberMin:       100,
			numberMax:       1000,
			findDuplicates:  false,
			evalConstExpr:   false,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 9, // All strings, 12345 should be filtered out (7 original + 2 from const_expressions.go)
		},
		{
			name:            "higher minimum occurrences",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         false,
			findDuplicates:  false,
			evalConstExpr:   false,
			minLength:       3,
			minOccurrences:  5, // higher than any string in our testdata
			expectedStrings: 1, // "test context" appears exactly 5 times
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(
				tt.path,
				"", // ignore
				"", // ignoreStrings
				tt.ignoreTests,
				tt.matchConstant,
				tt.numbers,
				tt.findDuplicates,
				tt.evalConstExpr,
				tt.numberMin,
				tt.numberMax,
				tt.minLength,
				tt.minOccurrences,
				map[Type]bool{},
			)

			strs, consts, err := p.ParseTree()
			if err != nil {
				t.Fatalf("ParseTree() error = %v", err)
			}

			if len(strs) != tt.expectedStrings {
				t.Errorf("ParseTree() found %d strings, want %d", len(strs), tt.expectedStrings)
				for str, occurrences := range strs {
					t.Logf("Found: %q with %d occurrences", str, len(occurrences))
				}
			}

			// Verify constant matches if expected
			if tt.expectedMatches != nil {
				for str, wantConst := range tt.expectedMatches {
					foundConsts, ok := consts[str]
					if !ok {
						t.Errorf("String %q not found in constants map", str)
						continue
					}
					if len(foundConsts) == 0 {
						t.Errorf("No constants found for string %q", str)
						continue
					}
					if foundConsts[0].Name != wantConst {
						t.Errorf("String %q matched with constant %q, want %q",
							str, foundConsts[0].Name, wantConst)
					}
				}
			}
		})
	}
}

func TestIntegrationExcludeTypes(t *testing.T) {
	tests := []struct {
		name            string
		excludeTypes    map[Type]bool
		expectedStrings int
	}{
		{
			name:            "no exclusions",
			excludeTypes:    map[Type]bool{},
			expectedStrings: 9, // All strings that appear at least twice (7 original + 2 from const_expressions.go)
		},
		{
			name:            "exclude assignments",
			excludeTypes:    map[Type]bool{Assignment: true},
			expectedStrings: 3, // After excluding assignments
		},
		{
			name: "exclude all types",
			excludeTypes: map[Type]bool{
				Assignment: true,
				Binary:     true,
				Case:       true,
				Return:     true,
				Call:       true,
			},
			expectedStrings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(
				"testdata",
				"",    // ignore
				"",    // ignoreStrings
				false, // ignoreTests
				false, // matchConstant
				false, // numbers
				false, // findDuplicates
				false, // evalConstExpressions
				0,     // numberMin
				0,     // numberMax
				3,     // minLength
				2,     // minOccurrences
				tt.excludeTypes,
			)

			strs, _, err := p.ParseTree()
			if err != nil {
				t.Fatalf("ParseTree() error = %v", err)
			}

			if len(strs) != tt.expectedStrings {
				t.Errorf("ParseTree() found %d strings, want %d", len(strs), tt.expectedStrings)
				for str, occurrences := range strs {
					t.Logf("Found: %q with %d occurrences", str, len(occurrences))
				}
			}
		})
	}
}
