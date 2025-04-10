package goconst

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func BenchmarkParseSampleFile(b *testing.B) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "testdata/sample.go", nil, 0)
	if err != nil {
		b.Fatalf("Failed to parse test file: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := &Parser{
			minLength:            3,
			minOccurrences:       2,
			supportedTokens:      []token.Token{token.STRING},
			excludeTypes:         map[Type]bool{},
			strs:                 Strings{},
			consts:               Constants{},
			matchConstant:        true,
			evalConstExpressions: false, // Disable for benchmark
			stringCount:          make(map[string]int),
			stringMutex:          sync.RWMutex{},
			stringCountMutex:     sync.RWMutex{},
			constMutex:           sync.RWMutex{},
		}

		v := &treeVisitor{
			p:           p,
			fileSet:     fset,
			packageName: "testdata",
		}

		ast.Walk(v, f)
		p.ProcessResults()
	}
}

func BenchmarkRun(b *testing.B) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "testdata/sample.go", nil, 0)
	if err != nil {
		b.Fatalf("Failed to parse test file: %v", err)
	}

	config := &Config{
		MinStringLength:      3,
		MinOccurrences:       2,
		MatchWithConstants:   true,
		EvalConstExpressions: false, // Disable for benchmark
	}

	chkr, info := checker(fset)
	_ = chkr.Files([]*ast.File{f})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := Run([]*ast.File{f}, fset, info, config)
		if err != nil {
			b.Fatalf("Run() error = %v", err)
		}
	}
}

func BenchmarkParseTree(b *testing.B) {
	b.Run("basic", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := New(
				"testdata",
				"",    // ignore
				"",    // ignoreStrings
				false, // ignoreTests
				false, // matchConstant
				false, // numbers
				true,  // findDuplicates
				false, // evalConstExpressions
				0,     // numberMin
				0,     // numberMax
				3,     // minLength
				2,     // minOccurrences
				map[Type]bool{},
			)

			_, _, err := p.ParseTree()
			if err != nil {
				b.Fatalf("ParseTree() error = %v", err)
			}
		}
	})

	b.Run("with-numbers", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := New(
				"testdata",
				"",    // ignore
				"",    // ignoreStrings
				false, // ignoreTests
				false, // matchConstant
				true,  // numbers
				true,  // findDuplicates
				false, // evalConstExpressions
				0,     // numberMin
				0,     // numberMax
				3,     // minLength
				2,     // minOccurrences
				map[Type]bool{},
			)

			_, _, err := p.ParseTree()
			if err != nil {
				b.Fatalf("ParseTree() error = %v", err)
			}
		}
	})

	b.Run("with-constants", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := New(
				"testdata",
				"",    // ignore
				"",    // ignoreStrings
				false, // ignoreTests
				true,  // matchConstant
				false, // numbers
				true,  // findDuplicates
				false, // evalConstExpressions
				0,     // numberMin
				0,     // numberMax
				3,     // minLength
				2,     // minOccurrences
				map[Type]bool{},
			)

			_, _, err := p.ParseTree()
			if err != nil {
				b.Fatalf("ParseTree() error = %v", err)
			}
		}
	})
}

// BenchmarkParallelProcessing2 tests the parallel implementation with various concurrency levels.
func BenchmarkParallelProcessing2(b *testing.B) {
	// Test with different concurrency levels
	concurrencyLevels := []int{1, 2, 4, 8, runtime.NumCPU()}

	for _, level := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency-%d", level), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				p := New(
					"testdata",
					"",
					"",
					false,
					false,
					true,
					true,
					false, // evalConstExpressions
					0,
					0,
					3,
					2,
					nil,
				)

				// Set the concurrency level
				p.SetConcurrency(level)

				// Parse the tree
				_, _, err := p.ParseTree()
				if err != nil {
					b.Fatalf("parse failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkParseDirectoryParallel tests the performance of parallel parsing on a directory
// with multiple files.
func BenchmarkParseDirectoryParallel(b *testing.B) {
	// Create a temporary directory with multiple files for testing
	tempDir, err := os.MkdirTemp("", "goconst-benchmark")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			b.Errorf("Failed to remove temp dir: %v", err)
		}
	}()

	// Create subdirectories to test recursive processing
	subdirCount := 5
	filesPerDir := 20

	// Generate a larger content template with more duplicate strings
	contentTemplate := `package benchmark

import (
	"fmt"
	"strings"
)

// Common constants that might be detected
const (
	CommonValue = "common value"
	OtherValue  = 42
)

// Function with duplicated strings and numbers
func function%d() {
	// Repeated string literal
	a := "hello world"
	b := "hello world"
	c := "hello world"
	
	// Unique string for this file
	unique := "unique string %d"
	
	// Repeated numbers
	n1 := 42
	n2 := 42
	n3 := 42
	
	// String comparison
	if a == "test string" {
		return
	}
	
	// Another repeated string
	value1 := "duplicate value"
	value2 := "duplicate value"
	
	// Switch with repeated cases
	switch b {
	case "case string":
		println("found")
	case "other case":
		println("found")
	case "case string": // Intentional duplicate
		println("found again")
	}
	
	// Function calls with repeated strings
	println("hello world")
	fmt.Println("hello world")
	fmt.Printf("Format: %%s", "hello world")
	
	// More repeated values
	m := make(map[string]string)
	m["key"] = "value"
	m["another"] = "value"
	
	// Binary expressions with repeated strings
	if strings.HasPrefix(unique, "prefix") {
		println("has prefix")
	}
	
	if strings.HasPrefix(unique, "prefix") {
		println("checking again")
	}
}

// Another function with duplicates
func helperFunction%d() string {
	// More duplicates across functions
	return "hello world"
}
`

	// Create a more complex directory structure
	for i := 0; i < subdirCount; i++ {
		// Create subdirectory
		subdir := filepath.Join(tempDir, fmt.Sprintf("subdir%d", i))
		if err := os.Mkdir(subdir, 0755); err != nil {
			b.Fatalf("Failed to create subdir: %v", err)
		}

		// Create files in the subdirectory
		for j := 0; j < filesPerDir; j++ {
			content := fmt.Sprintf(contentTemplate, j, j, j)

			filename := filepath.Join(subdir, fmt.Sprintf("file%d.go", j))
			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				b.Fatalf("Failed to write file: %v", err)
			}
		}
	}

	// Add the recursive notation to the path
	recursivePath := tempDir + "/..."

	b.ResetTimer() // Reset the timer after setup

	// Benchmark with sequential processing
	b.Run("Sequential", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := New(
				recursivePath,
				"",
				"",
				false,
				false,
				true,
				true,
				false, // evalConstExpressions
				0,
				0,
				3,
				2,
				nil,
			)

			// Force sequential processing
			p.SetConcurrency(1)

			// Parse the tree
			_, _, err := p.ParseTree()
			if err != nil {
				b.Fatalf("parse failed: %v", err)
			}
		}
	})

	// Parallel with max concurrency
	b.Run("Parallel", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			p := New(
				recursivePath,
				"",
				"",
				false,
				false,
				true,
				true,
				false, // evalConstExpressions
				0,
				0,
				3,
				2,
				nil,
			)

			// Parse the tree with default concurrency
			_, _, err := p.ParseTree()
			if err != nil {
				b.Fatalf("parse failed: %v", err)
			}
		}
	})
}

// BenchmarkFileReadingPerformance benchmarks the optimized file reading implementation
// with different file sizes to measure performance characteristics.
func BenchmarkFileReadingPerformance(b *testing.B) {
	// Create benchmark files of different sizes
	sizes := []int{1000, 10000, 100000}
	for _, size := range sizes {
		// Create a temporary file
		content := generateRandomContent(size)
		tempFile, err := os.CreateTemp("", "goconst-benchmark")
		if err != nil {
			b.Fatalf("Failed to create temp file: %v", err)
		}
		fileName := tempFile.Name()
		if _, err := tempFile.Write(content); err != nil {
			b.Fatalf("Failed to write to temp file: %v", err)
		}
		if err := tempFile.Close(); err != nil {
			b.Fatalf("Failed to close temp file: %v", err)
		}

		// Clean up the temp file when benchmark is done
		defer func() {
			if err := os.Remove(fileName); err != nil {
				b.Errorf("Failed to remove temp file: %v", err)
			}
		}()

		// Benchmark the optimized file reading
		b.Run(fmt.Sprintf("OptimizedIO_%d", size), func(b *testing.B) {
			parser := New("", "", "", false, false, false, true, false, 0, 0, 3, 2, make(map[Type]bool))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := parser.readFileEfficiently(fileName)
				if err != nil {
					b.Fatalf("Error reading file: %v", err)
				}
			}
		})

		// Benchmark standard file reading for comparison
		b.Run(fmt.Sprintf("StandardIO_%d", size), func(b *testing.B) {
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := os.ReadFile(fileName)
				if err != nil {
					b.Fatalf("Error reading file: %v", err)
				}
			}
		})
	}
}

// generateRandomContent creates random Go-like content for benchmarking
func generateRandomContent(size int) []byte {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 \t\n{}[]().,;:\"'=-+*/&|<>!?_"
	content := make([]byte, size)

	// Use time-based seed for pseudo-randomness
	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	// Fill with random characters
	for i := 0; i < size; i++ {
		content[i] = chars[r.Intn(len(chars))]
	}

	// Add some common Go constructs to make it look like Go code
	if size > 100 {
		copy(content[:20], []byte("package main\n\nfunc "))
	}

	return content
}

func BenchmarkParseTreeMinimal(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Minimal configuration
		p := New(
			"testdata",
			"",
			"",
			false,
			false,
			false,
			false,
			false, // evalConstExpressions
			0,
			0,
			3,
			2,
			map[Type]bool{},
		)
		_, _, err := p.ParseTree()
		if err != nil {
			b.Fatalf("Error parsing tree: %v", err)
		}
	}
}

func BenchmarkParseTreeWithNumbers(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Numbers enabled
		p := New(
			"testdata",
			"",
			"",
			false,
			false,
			true, // Parse numbers
			false,
			false, // evalConstExpressions
			0,
			0,
			3,
			2,
			map[Type]bool{},
		)
		_, _, err := p.ParseTree()
		if err != nil {
			b.Fatalf("Error parsing tree: %v", err)
		}
	}
}

func BenchmarkParseTreeWithConstMatch(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		// Match constants enabled
		p := New(
			"testdata",
			"",
			"",
			false,
			true, // Match constants
			false,
			false,
			false, // evalConstExpressions
			0,
			0,
			3,
			2,
			map[Type]bool{},
		)
		_, _, err := p.ParseTree()
		if err != nil {
			b.Fatalf("Error parsing tree: %v", err)
		}
	}
}

// BenchmarkStringInterning benchmarks the performance improvement from string interning
func BenchmarkStringInterning(b *testing.B) {
	b.ReportAllocs()

	// Generate some test data
	testData := make([]string, 100)
	for i := 0; i < 100; i++ {
		// Create strings that will sometimes be duplicates
		testData[i] = fmt.Sprintf("test-string-%d", i%20)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := New(
			"",
			"",
			"",
			false,
			false,
			false,
			false,
			false, // evalConstExpressions
			0,
			0,
			3,
			2,
			nil,
		)

		// Simulate processing these strings
		for _, s := range testData {
			// Intern the string
			interned := InternString(s)

			// Do something with the interned string to prevent optimization
			if len(interned) > 0 {
				p.stringCount[interned]++
			}
		}
	}
}

// BenchmarkParseTreeLargeCodebase benchmarks parsing a larger codebase
func BenchmarkParseTreeLargeCodebase(b *testing.B) {
	// Skip if not running in CI or explicitly requested with BENCH_LARGE=1
	if os.Getenv("CI") != "true" && os.Getenv("BENCH_LARGE") != "1" {
		b.Skip("Skipping large benchmark; run with BENCH_LARGE=1 to enable")
	}

	// Use the parent directory of the current workspace as test data
	// This gives us a real-world codebase to analyze
	wd, err := os.Getwd()
	if err != nil {
		b.Fatalf("Failed to get working directory: %v", err)
	}

	// Go up one level to get parent directory
	testPath := filepath.Dir(wd)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := New(
			testPath,
			"",
			"",
			true, // Ignore tests to reduce volume
			false,
			false,
			false,
			false, // evalConstExpressions
			0,
			0,
			3,
			2,
			nil, // No type exclusions
		)
		_, _, err := p.ParseTree()
		if err != nil {
			b.Fatalf("Error parsing tree: %v", err)
		}
	}
}

// BenchmarkStringPooling benchmarks the performance impact of string pooling
func BenchmarkStringPooling(b *testing.B) {
	b.ReportAllocs()

	// Create a set of strings to process with some duplication
	testStrings := make([]string, 10000)
	for i := 0; i < len(testStrings); i++ {
		testStrings[i] = fmt.Sprintf("test-string-%d", i%500)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		p := New(
			"",
			"",
			"",
			false,
			false,
			false,
			false,
			false, // evalConstExpressions
			0,
			0,
			3,
			2,
			nil, // No type exclusions
		)

		// Simulate processing all strings
		for _, s := range testStrings {
			// Use intern to ensure string deduplication
			internedString := InternString(s)
			p.stringCount[internedString]++

			// Simulate position tracking (simplified)
			if _, ok := p.strs[internedString]; !ok {
				p.strs[internedString] = make([]ExtendedPos, 0, 4)
			}
		}
	}
}

// BenchmarkParallelProcessing benchmarks the performance of parallel file processing
func BenchmarkParallelProcessing(b *testing.B) {
	// Use the testdata directory which should have multiple files
	testPath := filepath.Join(".", "testdata")

	// Ensure the test directory exists
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		b.Skipf("Test data directory %q does not exist", testPath)
	}

	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		p := New(
			testPath,
			"",
			"",
			false,
			false,
			false,
			false,
			false, // evalConstExpressions
			0,
			0,
			3,
			2,
			map[Type]bool{},
		)

		// Set the concurrency level
		p.SetConcurrency(runtime.NumCPU())

		b.StartTimer()
		_, _, err := p.ParseTree()
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
	}
}
