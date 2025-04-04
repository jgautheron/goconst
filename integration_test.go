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
		minOccurrences  int
		expectedStrings int
	}{
		{
			name:            "basic duplicate string detection",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         false,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 3, // "should be constant", "another duplicate", "test context"
		},
		{
			name:            "match with constants",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   true,
			numbers:         false,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 3, // same strings, but one matches a constant
		},
		{
			name:            "include numbers",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         true,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 4, // the 3 strings + "12345"
		},
		{
			name:            "filter by number range",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         true,
			numberMin:       100,
			numberMax:       1000,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 3, // 12345 should be filtered out
		},
		{
			name:            "higher minimum occurrences",
			path:            "testdata",
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         false,
			minLength:       3,
			minOccurrences:  5, // higher than any string in our testdata
			expectedStrings: 1, // "test context" appears 5 times
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
				tt.numberMin,
				tt.numberMax,
				tt.minLength,
				tt.minOccurrences,
				map[Type]bool{},
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

func TestIntegrationExcludeTypes(t *testing.T) {
	tests := []struct {
		name            string
		excludeTypes    map[Type]bool
		expectedStrings int
	}{
		{
			name:            "no exclusions",
			excludeTypes:    map[Type]bool{},
			expectedStrings: 3,
		},
		{
			name:            "exclude assignments",
			excludeTypes:    map[Type]bool{Assignment: true},
			expectedStrings: 1, // After excluding assignments, only "test context" remains
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
