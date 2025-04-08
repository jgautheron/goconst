package goconst

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestParser_ProcessResults(t *testing.T) {
	tests := []struct {
		name           string
		strs           Strings
		stringCount    map[string]int
		ignoreStrings  string
		minOccurrences int
		numberMin      int
		numberMax      int
		wantRemaining  int
	}{
		{
			name: "basic filtering by occurrences",
			strs: Strings{
				"test1": []ExtendedPos{
					{}, {}, // 2 occurrences
				},
				"test2": []ExtendedPos{
					{}, // 1 occurrence
				},
			},
			stringCount: map[string]int{
				"test1": 2,
				"test2": 1,
			},
			minOccurrences: 2,
			wantRemaining:  1, // only "test1" should remain
		},
		{
			name: "filtering by regex",
			strs: Strings{
				"test": []ExtendedPos{
					{}, {}, // 2 occurrences
				},
				"production": []ExtendedPos{
					{}, {}, // 2 occurrences
				},
			},
			stringCount: map[string]int{
				"test":       2,
				"production": 2,
			},
			ignoreStrings:  "test",
			minOccurrences: 2,
			wantRemaining:  1, // only "production" should remain
		},
		{
			name: "filtering by number min",
			strs: Strings{
				"100": []ExtendedPos{
					{}, {}, // 2 occurrences
				},
				"200": []ExtendedPos{
					{}, {}, // 2 occurrences
				},
			},
			stringCount: map[string]int{
				"100": 2,
				"200": 2,
			},
			numberMin:      150,
			minOccurrences: 2,
			wantRemaining:  1, // only "200" should remain
		},
		{
			name: "filtering by number max",
			strs: Strings{
				"100": []ExtendedPos{
					{}, {}, // 2 occurrences
				},
				"200": []ExtendedPos{
					{}, {}, // 2 occurrences
				},
			},
			stringCount: map[string]int{
				"100": 2,
				"200": 2,
			},
			numberMax:      150,
			minOccurrences: 2,
			wantRemaining:  1, // only "100" should remain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Parser{
				strs:           tt.strs,
				stringCount:    tt.stringCount,
				ignoreStrings:  tt.ignoreStrings,
				minOccurrences: tt.minOccurrences,
				numberMin:      tt.numberMin,
				numberMax:      tt.numberMax,
				// Initialize stringCount mutex to prevent nil panic
				stringCountMutex: sync.RWMutex{},
				stringMutex:      sync.RWMutex{},
			}

			// Initialize the regexes if ignoreStrings is set
			if tt.ignoreStrings != "" {
				var err error
				p.ignoreStringsRegex, err = regexp.Compile(tt.ignoreStrings)
				if err != nil {
					t.Fatalf("Failed to compile regex: %v", err)
				}
			}

			p.ProcessResults()

			if len(p.strs) != tt.wantRemaining {
				t.Errorf("ProcessResults() = %v remaining strings, want %v", len(p.strs), tt.wantRemaining)
			}
		})
	}
}

func TestParser_New(t *testing.T) {
	// Test that the New function sets up the parser correctly
	p := New(
		"testPath",
		"testIgnore",
		"testIgnoreStrings",
		true,  // ignoreTests
		true,  // matchConstant
		true,  // numbers
		true,  // findDuplicates
		false, // evalConstExpressions
		100,   // numberMin
		500,   // numberMax
		3,     // minLength
		2,     // minOccurrences
		map[Type]bool{Assignment: true},
	)

	// Verify that all parameters were set correctly
	if p.path != "testPath" {
		t.Errorf("New() path = %v, want %v", p.path, "testPath")
	}
	if p.ignore != "testIgnore" {
		t.Errorf("New() ignore = %v, want %v", p.ignore, "testIgnore")
	}
	if p.ignoreStrings != "testIgnoreStrings" {
		t.Errorf("New() ignoreStrings = %v, want %v", p.ignoreStrings, "testIgnoreStrings")
	}
	if !p.ignoreTests {
		t.Errorf("New() ignoreTests = %v, want %v", p.ignoreTests, true)
	}
	if !p.matchConstant {
		t.Errorf("New() matchConstant = %v, want %v", p.matchConstant, true)
	}
	if !p.findDuplicates {
		t.Errorf("New() findDuplicates %v, want %v", p.findDuplicates, true)
	}
	if p.minLength != 3 {
		t.Errorf("New() minLength = %v, want %v", p.minLength, 3)
	}
	if p.minOccurrences != 2 {
		t.Errorf("New() minOccurrences = %v, want %v", p.minOccurrences, 2)
	}
	if p.numberMin != 100 {
		t.Errorf("New() numberMin = %v, want %v", p.numberMin, 100)
	}
	if p.numberMax != 500 {
		t.Errorf("New() numberMax = %v, want %v", p.numberMax, 500)
	}
	if !p.excludeTypes[Assignment] {
		t.Errorf("New() excludeTypes[Assignment] = %v, want %v", p.excludeTypes[Assignment], true)
	}

	// Verify that supportedTokens includes both STRING and numeric tokens
	foundString := false
	foundInt := false
	foundFloat := false
	for _, t := range p.supportedTokens {
		if t == token.STRING {
			foundString = true
		}
		if t == token.INT {
			foundInt = true
		}
		if t == token.FLOAT {
			foundFloat = true
		}
	}

	if !foundString {
		t.Error("New() supportedTokens does not include STRING token")
	}
	if !foundInt {
		t.Error("New() supportedTokens does not include INT token")
	}
	if !foundFloat {
		t.Error("New() supportedTokens does not include FLOAT token")
	}
}

func TestParseTree(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "goconst-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create a test file with known constants and repeated strings
	testFile := filepath.Join(tempDir, "test.go")
	testContent := `package test
const ExistingConst = "constant"
func test() {
	a := "repeated"
	b := "repeated"
	c := 123
	d := 123
}`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Test cases for ParseTree
	tests := []struct {
		name            string
		path            string
		ignoreTests     bool
		matchConstant   bool
		numbers         bool
		minLength       int
		minOccurrences  int
		expectedStrings int
	}{
		{
			name:            "basic parsing",
			path:            tempDir,
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         false,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 1, // "repeated" should be found
		},
		{
			name:            "with numbers",
			path:            tempDir,
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         true,
			minLength:       1,
			minOccurrences:  2,
			expectedStrings: 2, // "repeated" and "123" should be found
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
				false, // findDuplicates
				false, // evalConstExpressions
				0,     // numberMin
				0,     // numberMax
				tt.minLength,
				tt.minOccurrences,
				map[Type]bool{},
			)

			strs, _, err := p.ParseTree()
			if err != nil {
				t.Fatalf("ParseTree() error = %v", err)
			}

			if len(strs) != tt.expectedStrings {
				t.Errorf("ParseTree() found %d strings, expected %d", len(strs), tt.expectedStrings)
				for str, occurrences := range strs {
					t.Logf("Found: %q with %d occurrences", str, len(occurrences))
				}
			}
		})
	}

	// Test recursive directory parsing
	recursiveDir := filepath.Join(tempDir, "recursive")
	if err := os.Mkdir(recursiveDir, 0755); err != nil {
		t.Fatalf("Failed to create recursive directory: %v", err)
	}

	recursiveFile := filepath.Join(recursiveDir, "nested.go")
	recursiveContent := `package nested
func nested() {
	a := "nested"
	b := "nested"
}`
	if err := os.WriteFile(recursiveFile, []byte(recursiveContent), 0644); err != nil {
		t.Fatalf("Failed to write recursive test file: %v", err)
	}

	t.Run("recursive parsing", func(t *testing.T) {
		p := New(
			tempDir+"...", // Use recursive notation
			"",            // ignore
			"",            // ignoreStrings
			false,         // ignoreTests
			false,         // matchConstant
			false,         // numbers
			false,         // findDuplicates
			false,         // evalConstExpressions
			0,             // numberMin
			0,             // numberMax
			3,             // minLength
			2,             // minOccurrences
			map[Type]bool{},
		)

		strs, _, err := p.ParseTree()
		if err != nil {
			t.Fatalf("ParseTree() error = %v", err)
		}

		// Should find both "repeated" in root and "nested" in subdirectory
		if len(strs) != 2 {
			t.Errorf("ParseTree() found %d strings, expected 2", len(strs))
			for str, occurrences := range strs {
				t.Logf("Found: %q with %d occurrences", str, len(occurrences))
			}
		}
	})

	// Test ignoreTests flag
	testTestFile := filepath.Join(tempDir, "test_test.go")
	testTestContent := `package test
func TestFunction(t *testing.T) {
	a := "test_repeated"
	b := "test_repeated"
}`
	if err := os.WriteFile(testTestFile, []byte(testTestContent), 0644); err != nil {
		t.Fatalf("Failed to write test test file: %v", err)
	}

	ignoreTestsTests := []struct {
		name            string
		path            string
		ignoreTests     bool
		matchConstant   bool
		numbers         bool
		findDuplicates  bool
		minLength       int
		minOccurrences  int
		expectedStrings int
	}{
		{
			name:            "include test files",
			path:            tempDir,
			ignoreTests:     false,
			matchConstant:   false,
			numbers:         false,
			findDuplicates:  false,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 2, // Should find both "repeated" and "test_repeated"
		},
		{
			name:            "exclude test files",
			path:            tempDir,
			ignoreTests:     true,
			matchConstant:   false,
			numbers:         false,
			findDuplicates:  false,
			minLength:       3,
			minOccurrences:  2,
			expectedStrings: 1, // Should only find "repeated"
		},
	}

	for _, tt := range ignoreTestsTests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(
				tt.path,
				"", // ignore
				"", // ignoreStrings
				tt.ignoreTests,
				tt.matchConstant,
				tt.numbers,
				false, // findDuplicates
				false, // evalConstExpressions
				0,     // numberMin
				0,     // numberMax
				tt.minLength,
				tt.minOccurrences,
				map[Type]bool{},
			)

			strs, _, err := p.ParseTree()
			if err != nil {
				t.Fatalf("ParseTree() error = %v", err)
			}

			if len(strs) != tt.expectedStrings {
				t.Errorf("ParseTree() found %d strings, expected %d", len(strs), tt.expectedStrings)
				for str, occurrences := range strs {
					t.Logf("Found: %q with %d occurrences", str, len(occurrences))
				}
			}
		})
	}

	// Test ignore pattern
	ignoreFile := filepath.Join(tempDir, "ignore_me.go")
	ignoreContent := `package test
func ignored() {
	a := "ignore_repeated"
	b := "ignore_repeated"
}`
	if err := os.WriteFile(ignoreFile, []byte(ignoreContent), 0644); err != nil {
		t.Fatalf("Failed to write ignore test file: %v", err)
	}

	t.Run("test ignore pattern", func(t *testing.T) {
		p := New(
			tempDir,
			"ignore_me", // ignore files with "ignore_me" in the name
			"",          // ignoreStrings
			false,       // ignoreTests
			false,       // matchConstant
			false,       // numbers
			false,       // findDuplicates
			false,       // evalConstExpressions
			0,           // numberMin
			0,           // numberMax
			3,           // minLength
			2,           // minOccurrences
			map[Type]bool{},
		)

		strs, _, err := p.ParseTree()
		if err != nil {
			t.Fatalf("ParseTree() error = %v", err)
		}

		// Should not find "ignore_repeated" due to ignore pattern
		for str := range strs {
			if str == "ignore_repeated" {
				t.Errorf("ParseTree() found 'ignore_repeated' despite ignore pattern")
			}
		}
	})

	// Test computed string constants
	computedFile := filepath.Join(tempDir, "computed_values.go")
	computedContent := `package test
const Duplicate = "duplicate"
const DuplicateValue = Duplicate + " value"
func foo() {
	a := "duplicate value"
	b := "duplicate value"
}`
	if err := os.WriteFile(computedFile, []byte(computedContent), 0644); err != nil {
		t.Fatalf("Failed to write computed values file: %v", err)
	}

	t.Run("test computed values", func(t *testing.T) {
		p := New(
			tempDir,
			"",    // ignore
			"",    // ignoreStrings
			false, // ignoreTests
			false, // matchConstant
			true,  // numbers
			true,  // findDuplicates
			true,  // findDuplicates
			0,     // numberMin
			0,     // numberMax
			3,     // minLength
			2,     // minOccurrences
			map[Type]bool{},
		)

		_, csts, err := p.ParseTree()
		if err != nil {
			t.Fatalf("ParseTree() error = %v", err)
		}

		// Should find a constant with value "duplicate value"
		found := false
		for val, cst := range csts {
			if val == "duplicate value" {
				if len(cst) != 1 {
					t.Errorf("ParseTree() found %d constants with value 'duplicated value', expected 1", len(cst))
					continue
				}
				if cst[0].Name != "DuplicateValue" {
					t.Errorf("ParseTree() found const named %s to have value 'duplicate value', expected const named DuplicateValue", cst[0].Name)
				} else {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("ParseTree() did not find computed const DuplicateValue")
		}
	})

	// Test computed string constants
	computedNumsFile := filepath.Join(tempDir, "computed_numbers.go")
	computedNumsContent := `package test
const KiB = (1 << 10) + 0
const Kibibytes = 1024

func foo() {
  num := 1024
}
`
	if err := os.WriteFile(computedNumsFile, []byte(computedNumsContent), 0644); err != nil {
		t.Fatalf("Failed to write computed numbers file: %v", err)
	}

	t.Run("test computed numeric values", func(t *testing.T) {
		p := New(
			tempDir,
			"",    // ignore
			"",    // ignoreStrings
			false, // ignoreTests
			true,  // matchConstant
			true,  // numbers
			true,  // findDuplicates
			true,  // findDuplicates
			0,     // numberMin
			0,     // numberMax
			0,     // minLength
			1,     // minOccurrences
			map[Type]bool{},
		)

		_, csts, err := p.ParseTree()
		if err != nil {
			t.Fatalf("ParseTree() error = %v", err)
		}

		// Should find a constant with value "duplicate value"
		for val, cst := range csts {
			if val == "1024" {
				if len(cst) != 2 {
					t.Errorf("ParseTree() found %d constants with value '1024', expected 2", len(cst))
					continue
				}
			}
		}
	})

}

// BenchmarkFileTraversal tests the performance of different traversal strategies
func BenchmarkFileTraversal(b *testing.B) {
	// Create a temporary directory with a nested structure for benchmarking
	tempDir, err := os.MkdirTemp("", "goconst-bench")
	if err != nil {
		b.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	// Create a nested directory structure with test files
	createBenchmarkFiles(b, tempDir, 5, 10, 5)

	// Test the original implementation
	b.Run("Original", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := New(
				tempDir,
				"", // ignore
				"", // ignoreStrings
				false,
				false,
				true,
				false, // findDuplicates
				false, // evalConstExpressions
				0,
				0,
				3,
				2,
				map[Type]bool{},
			)
			// Use the default approach
			_, _, err := p.ParseTree()
			if err != nil {
				b.Fatalf("ParseTree() error = %v", err)
			}
		}
	})

	// Test the optimized concurrent file traversal
	b.Run("OptimizedConcurrent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := New(
				tempDir,
				"", // ignore
				"", // ignoreStrings
				false,
				false,
				true,
				false,
				false,
				0,
				0,
				3,
				2,
				map[Type]bool{},
			)

			// Force maximum concurrency for consistent benchmarking
			p.SetConcurrency(runtime.NumCPU())

			_, _, err := p.ParseTree()
			if err != nil {
				b.Fatalf("ParseTree() error = %v", err)
			}
		}
	})

	// Test the batch processing implementation
	b.Run("BatchProcessing", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := New(
				tempDir,
				"", // ignore
				"", // ignoreStrings
				false,
				false,
				true,
				false,
				false,
				0,
				0,
				3,
				2,
				map[Type]bool{},
			)

			// Enable batch processing with a small batch size for testing
			p.EnableBatchProcessing(10)

			_, _, err := p.ParseTree()
			if err != nil {
				b.Fatalf("ParseTree() error = %v", err)
			}
		}
	})

	// Test different batch sizes to find optimal settings
	batchSizes := []int{10, 50, 100, 500}
	for _, size := range batchSizes {
		b.Run(fmt.Sprintf("BatchSize_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				p := New(
					tempDir,
					"", // ignore
					"", // ignoreStrings
					false,
					false,
					true,
					false,
					false,
					0,
					0,
					3,
					2,
					map[Type]bool{},
				)

				p.EnableBatchProcessing(size)

				_, _, err := p.ParseTree()
				if err != nil {
					b.Fatalf("ParseTree() error = %v", err)
				}
			}
		})
	}
}

// createBenchmarkFiles creates a nested directory structure with Go files for benchmarking
// depth: how deep to nest directories
// width: how many subdirectories per level
// filesPerDir: how many Go files to create in each directory
func createBenchmarkFiles(b *testing.B, dir string, depth, width, filesPerDir int) {
	if depth <= 0 {
		return
	}

	// Create Go files in the current directory
	for i := 0; i < filesPerDir; i++ {
		filename := filepath.Join(dir, fmt.Sprintf("file%d.go", i))
		content := generateGoFile(i)
		err := os.WriteFile(filename, []byte(content), 0644)
		if err != nil {
			b.Fatalf("Failed to write benchmark file: %v", err)
		}
	}

	// Create subdirectories and recurse
	for i := 0; i < width; i++ {
		subdir := filepath.Join(dir, fmt.Sprintf("subdir%d", i))
		err := os.Mkdir(subdir, 0755)
		if err != nil {
			b.Fatalf("Failed to create benchmark directory: %v", err)
		}

		// Recurse with reduced depth
		createBenchmarkFiles(b, subdir, depth-1, width, filesPerDir)
	}
}

// generateGoFile creates a Go file with some repeated strings for benchmarking
func generateGoFile(fileIndex int) string {
	// Create strings that will be repeated across files
	commonStrings := []string{
		`"this is a common string"`,
		`"another common string"`,
		`"yet another string"`,
		`123`,
		`456`,
	}

	// Create some file-specific strings to ensure variety
	fileSpecificStrings := []string{
		fmt.Sprintf(`"file specific %d"`, fileIndex),
		fmt.Sprintf(`"another one %d"`, fileIndex),
		fmt.Sprintf(`%d`, 1000+fileIndex),
	}

	var b strings.Builder
	b.WriteString("package benchmark\n\n")

	// Add a constant definition
	b.WriteString(fmt.Sprintf("const FileConst%d = %s\n\n", fileIndex, commonStrings[fileIndex%len(commonStrings)]))

	// Create a function with repeated strings
	b.WriteString(fmt.Sprintf("func Function%d() {\n", fileIndex))

	// Add some variable assignments with repeated strings
	for i := 0; i < 5; i++ {
		stringIndex := (fileIndex + i) % len(commonStrings)
		b.WriteString(fmt.Sprintf("\tvar%d := %s\n", i, commonStrings[stringIndex]))
	}

	// Add some file-specific strings
	for i := 0; i < len(fileSpecificStrings); i++ {
		b.WriteString(fmt.Sprintf("\tspecific%d := %s\n", i, fileSpecificStrings[i]))
	}

	// Add some conditions with repeated strings
	b.WriteString("\tif x == " + commonStrings[0] + " {\n")
	b.WriteString("\t\treturn\n")
	b.WriteString("\t}\n")

	// Add a switch statement with repeated strings
	b.WriteString("\tswitch y {\n")
	for i := 0; i < 3; i++ {
		stringIndex := (fileIndex + i) % len(commonStrings)
		b.WriteString(fmt.Sprintf("\tcase %s:\n", commonStrings[stringIndex]))
		b.WriteString(fmt.Sprintf("\t\tfmt.Println(%s)\n", fileSpecificStrings[i%len(fileSpecificStrings)]))
	}
	b.WriteString("\t}\n")

	b.WriteString("}\n")

	return b.String()
}

// BenchmarkFileReading tests the performance of the optimized file reading
func BenchmarkFileReading(b *testing.B) {
	// Create temporary files of different sizes for benchmarking
	testSizes := []int{1000, 10000, 100000}
	tempFiles := make([]string, len(testSizes))

	for i, size := range testSizes {
		tempFile, err := os.CreateTemp("", "goconst-bench-*.go")
		if err != nil {
			b.Fatalf("Failed to create temp file: %v", err)
		}
		defer func() {
			if err := os.Remove(tempFile.Name()); err != nil {
				b.Errorf("Failed to remove temp file: %v", err)
			}
		}()
		tempFiles[i] = tempFile.Name()

		// Write a large Go file for testing
		content := generateLargeGoFile(size) // Size in lines
		if _, err := tempFile.Write([]byte(content)); err != nil {
			b.Fatalf("Failed to write to temp file: %v", err)
		}
		if err := tempFile.Close(); err != nil {
			b.Fatalf("Failed to close temp file: %v", err)
		}
	}

	// Test standard file reading with different file sizes
	for i, size := range testSizes {
		b.Run(fmt.Sprintf("StandardIO_%d", size), func(b *testing.B) {
			for j := 0; j < b.N; j++ {
				_, err := os.ReadFile(tempFiles[i])
				if err != nil {
					b.Fatalf("ReadFile() error = %v", err)
				}
			}
		})
	}

	// Test optimized file reading with different file sizes
	for i, size := range testSizes {
		b.Run(fmt.Sprintf("OptimizedIO_%d", size), func(b *testing.B) {
			p := New("", "", "", false, false, false, false, false, 0, 0, 3, 2, map[Type]bool{})

			b.ResetTimer()
			for j := 0; j < b.N; j++ {
				_, err := p.readFileEfficiently(tempFiles[i])
				if err != nil {
					b.Fatalf("readFileEfficiently() error = %v", err)
				}
			}
		})
	}
}

// generateLargeGoFile creates a large Go file with many functions and strings
func generateLargeGoFile(lineCount int) string {
	var b strings.Builder
	b.WriteString("package benchmark\n\n")

	// Add imports
	b.WriteString("import (\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"strings\"\n")
	b.WriteString(")\n\n")

	// Add constants
	b.WriteString("const (\n")
	for i := 0; i < 10; i++ {
		b.WriteString(fmt.Sprintf("\tConst%d = \"constant value %d\"\n", i, i))
	}
	b.WriteString(")\n\n")

	// Add functions to reach the desired line count
	functionsNeeded := (lineCount - 20) / 10 // Rough estimate
	for i := 0; i < functionsNeeded; i++ {
		b.WriteString(fmt.Sprintf("func BenchFunction%d() string {\n", i))
		b.WriteString("\tvar result strings.Builder\n")

		// Add some repeated strings
		for j := 0; j < 5; j++ {
			b.WriteString(fmt.Sprintf("\tstr%d := \"repeated string %d\"\n", j, j%3))
		}

		// Add some conditions
		b.WriteString("\tif len(result.String()) > 0 {\n")
		b.WriteString("\t\treturn \"non-empty\"\n")
		b.WriteString("\t}\n")

		b.WriteString("\treturn result.String()\n")
		b.WriteString("}\n\n")
	}

	return b.String()
}
