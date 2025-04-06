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
			minLength:        3,
			minOccurrences:   2,
			supportedTokens:  []token.Token{token.STRING},
			excludeTypes:     map[Type]bool{},
			strs:             Strings{},
			consts:           Constants{},
			matchConstant:    true,
			stringCount:      make(map[string]int),
			stringMutex:      sync.RWMutex{},
			stringCountMutex: sync.RWMutex{},
			constMutex:       sync.RWMutex{},
		}

		v := &treeVisitor{
			p:           p,
			fileSet:     fset,
			packageName: "testdata",
			fileName:    "testdata/sample.go",
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
		MinStringLength:    3,
		MinOccurrences:     2,
		MatchWithConstants: true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Run([]*ast.File{f}, fset, config)
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

// BenchmarkParallelProcessing tests the parallel implementation with various concurrency levels.
func BenchmarkParallelProcessing(b *testing.B) {
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
	defer os.RemoveAll(tempDir)

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
		tempFile.Close()

		// Clean up the temp file when benchmark is done
		defer os.Remove(fileName)

		// Benchmark the optimized file reading
		b.Run(fmt.Sprintf("OptimizedIO_%d", size), func(b *testing.B) {
			parser := New("", "", "", false, false, false, 0, 0, 3, 2, make(map[Type]bool))
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
