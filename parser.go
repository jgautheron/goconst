// Package goconst finds repeated strings that could be replaced by a constant.
//
// There are obvious benefits to using constants instead of repeating strings,
// mostly to ease maintenance. Cannot argue against changing a single constant versus many strings.
// While this could be considered a beginner mistake, across time,
// multiple packages and large codebases, some repetition could have slipped in.
package goconst

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// StringBuilderPool is a pool of string builders to reduce memory allocations
var StringBuilderPool = sync.Pool{
	New: func() interface{} {
		return new(strings.Builder)
	},
}

// FileReaderPool is a pool of byte buffers used for reading files
var FileReaderPool = sync.Pool{
	New: func() interface{} {
		// Start with a 32KB buffer, which is sufficient for most Go files
		return make([]byte, 32*1024)
	},
}

// ByteBufferPool is a pool for temporary byte slices
var ByteBufferPool = sync.Pool{
	New: func() interface{} {
		// 8KB initial capacity is a good balance
		return make([]byte, 0, 8*1024)
	},
}

// StringBufferPool is a pool for string slices
var StringBufferPool = sync.Pool{
	New: func() interface{} {
		return make([]string, 0, 32)
	},
}

// ExtendedPosPool is a pool for slices of ExtendedPos
var ExtendedPosPool = sync.Pool{
	New: func() interface{} {
		return make([]ExtendedPos, 0, 8)
	},
}

// StringInternPool is a pool for deduplicating strings to reduce memory usage
var StringInternPool = sync.Map{}

// InternString returns a deduplicated reference to the given string
// to reduce memory usage when the same string appears multiple times
func InternString(s string) string {
	if s == "" {
		return ""
	}

	if interned, ok := StringInternPool.Load(s); ok {
		return interned.(string)
	}
	// Store a copy to prevent external modifications
	interned := string([]byte(s))
	StringInternPool.Store(interned, interned)
	return interned
}

// GetStringBuilder retrieves a string builder from the pool
func GetStringBuilder() *strings.Builder {
	return StringBuilderPool.Get().(*strings.Builder)
}

// PutStringBuilder returns a string builder to the pool after resetting it
func PutStringBuilder(sb *strings.Builder) {
	sb.Reset()
	StringBuilderPool.Put(sb)
}

// GetByteBuffer retrieves a byte buffer from the pool
func GetByteBuffer() []byte {
	return ByteBufferPool.Get().([]byte)[:0] // Reset length but keep capacity
}

// PutByteBuffer returns a byte buffer to the pool
func PutByteBuffer(buf []byte) {
	bufCopy := make([]byte, 0, cap(buf))
	ByteBufferPool.Put(&bufCopy)
}

// GetStringBuffer retrieves a string slice from the pool
func GetStringBuffer() []string {
	return StringBufferPool.Get().([]string)[:0] // Reset length but keep capacity
}

// PutStringBuffer returns a string slice to the pool
func PutStringBuffer(slice []string) {
	sliceCopy := make([]string, 0, cap(slice))
	StringBufferPool.Put(sliceCopy)
}

// GetExtendedPosBuffer retrieves an ExtendedPos slice from the pool
func GetExtendedPosBuffer() []ExtendedPos {
	return ExtendedPosPool.Get().([]ExtendedPos)[:0] // Reset length but keep capacity
}

// PutExtendedPosBuffer returns an ExtendedPos slice to the pool
func PutExtendedPosBuffer(slice []ExtendedPos) {
	sliceCopy := make([]ExtendedPos, 0, cap(slice))
	ExtendedPosPool.Put(&sliceCopy)
}

const (
	testSuffix = "_test.go"
)

// Parser represents the core analysis engine for finding repeated strings and constants.
// It holds both configuration options and the internal state during analysis.
type Parser struct {
	// Meant to be passed via New()
	path, ignore, ignoreStrings string
	ignoreTests, matchConstant  bool
	minLength, minOccurrences   int
	numberMin, numberMax        int
	excludeTypes                map[Type]bool
	maxConcurrency              int

	supportedTokens []token.Token

	// Internals
	strs        Strings
	consts      Constants
	stringMutex sync.RWMutex
	constMutex  sync.RWMutex

	// Pre-compiled regexes for efficiency
	ignoreRegex        *regexp.Regexp
	ignoreStringsRegex *regexp.Regexp

	// String occurrence counter
	// Using a separate counter map improves performance for
	// tracking frequency without having to compute len(items) repeatedly
	stringCount      map[string]int
	stringCountMutex sync.RWMutex

	// Batch processing options
	batchSize      int
	enableBatching bool

	// FileSet cache to avoid creating multiple fileSets
	fileSetCache *token.FileSet
	fileSetMutex sync.Mutex
}

// New creates a new instance of the parser.
// This is your entry point if you'd like to use goconst as an API.
//
// Parameters:
//   - path: the file or directory path to analyze
//   - ignore: regex pattern to ignore files
//   - ignoreStrings: regex pattern to ignore strings
//   - ignoreTests: whether to ignore test files
//   - matchConstant: whether to match strings with existing constants
//   - numbers: whether to analyze number literals
//   - numberMin/numberMax: range limits for number analysis
//   - minLength: minimum string length to consider
//   - minOccurrences: minimum occurrences to report
//   - excludeTypes: map of context types to exclude
func New(path, ignore, ignoreStrings string, ignoreTests, matchConstant, numbers bool, numberMin, numberMax, minLength, minOccurrences int, excludeTypes map[Type]bool) *Parser {
	supportedTokens := []token.Token{token.STRING}
	if numbers {
		supportedTokens = append(supportedTokens, token.INT, token.FLOAT)
	}

	// Set default concurrency to number of CPUs
	maxConcurrency := runtime.NumCPU()

	// Pre-compile regular expressions for efficiency
	var ignoreRegex, ignoreStringsRegex *regexp.Regexp
	var err error

	if ignore != "" {
		ignoreRegex, err = regexp.Compile(ignore)
		if err != nil {
			log.Printf("Warning: Invalid ignore regex pattern '%s': %v", ignore, err)
		}
	}

	if ignoreStrings != "" {
		ignoreStringsRegex, err = regexp.Compile(ignoreStrings)
		if err != nil {
			log.Printf("Warning: Invalid ignoreStrings regex pattern '%s': %v", ignoreStrings, err)
		}
	}

	// Estimate capacity based on typical usage patterns
	stringMapCapacity := 500
	constMapCapacity := 100

	// For large codebases, increase capacity estimates
	if numbers {
		stringMapCapacity *= 2 // Numbers typically increase the result set
	}

	// Intern common strings to reduce memory usage
	path = InternString(path)
	ignore = InternString(ignore)
	ignoreStrings = InternString(ignoreStrings)

	// Create a single FileSet to be reused
	fileSet := token.NewFileSet()

	return &Parser{
		path:               path,
		ignore:             ignore,
		ignoreStrings:      ignoreStrings,
		ignoreTests:        ignoreTests,
		matchConstant:      matchConstant,
		minLength:          minLength,
		minOccurrences:     minOccurrences,
		numberMin:          numberMin,
		numberMax:          numberMax,
		supportedTokens:    supportedTokens,
		excludeTypes:       excludeTypes,
		maxConcurrency:     maxConcurrency,
		ignoreRegex:        ignoreRegex,
		ignoreStringsRegex: ignoreStringsRegex,

		// Initialize the maps with capacity hints
		strs:        make(Strings, stringMapCapacity),
		consts:      make(Constants, constMapCapacity),
		stringCount: make(map[string]int, stringMapCapacity),

		// Default batch processing settings
		batchSize:      1000,
		enableBatching: false,

		// Cache a single FileSet for reuse
		fileSetCache: fileSet,
	}
}

// SetConcurrency allows setting the maximum number of goroutines to use
// for parallel file processing. Default is the number of CPUs.
func (p *Parser) SetConcurrency(max int) {
	if max > 0 {
		p.maxConcurrency = max
	}
}

// EnableBatchProcessing activates batch processing mode for very large codebases.
// This mode collects files in batches before processing them to reduce memory usage.
// The batchSize parameter controls how many files to process in each batch.
func (p *Parser) EnableBatchProcessing(batchSize int) {
	p.enableBatching = true
	if batchSize > 0 {
		p.batchSize = batchSize
	}
}

// ParseTree will search the given path for occurrences that could be moved into constants.
// If "..." is appended, the search will be recursive.
//
// It returns maps of strings and constants found during the analysis, and any error encountered.
// Use ProcessResults to filter the results based on configuration before retrieving them.
func (p *Parser) ParseTree() (Strings, Constants, error) {
	pathLen := len(p.path)
	// Parse recursively the given path if the recursive notation is found
	if pathLen >= 5 && p.path[pathLen-3:] == "..." {
		return p.parseTreeConcurrent(p.path[:pathLen-3], true)
	} else {
		return p.parseTreeConcurrent(p.path, false)
	}
}

// parseTreeConcurrent implements an optimized concurrent file traversal
// that efficiently processes directories and files using worker pools.
func (p *Parser) parseTreeConcurrent(rootPath string, recursive bool) (Strings, Constants, error) {
	// If batch processing is enabled, use that implementation instead
	if p.enableBatching {
		return p.parseTreeBatched(rootPath, recursive)
	}

	// Process files directly if the input is a single file
	fi, err := os.Stat(rootPath)
	if err == nil && !fi.IsDir() {
		fset := p.getFileSet()
		src, err := p.readFileEfficiently(rootPath)
		if err != nil {
			return nil, nil, err
		}

		f, err := parser.ParseFile(fset, rootPath, src, 0)
		if err != nil {
			return nil, nil, err
		}

		// Process the file
		ast.Walk(&treeVisitor{
			fileSet:     fset,
			packageName: f.Name.Name,
			fileName:    rootPath,
			p:           p,
			ignoreRegex: p.ignoreStringsRegex,
		}, f)

		// Post-process and filter results
		p.ProcessResults()
		return p.strs, p.consts, nil
	}

	// Create a channel to collect all files to be processed
	filesChan := make(chan string, 1000)

	// Start a goroutine to collect all Go files
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(filesChan)

		// If not recursive, just handle a single directory
		if !recursive {
			entries, err := os.ReadDir(rootPath)
			if err != nil {
				log.Printf("Error reading directory %s: %v", rootPath, err)
				return
			}

			// Process entries
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}

				path := filepath.Join(rootPath, entry.Name())
				if strings.HasSuffix(path, ".go") {
					// Skip test files if configured
					if p.ignoreTests && strings.HasSuffix(path, testSuffix) {
						continue
					}

					// Skip files matching ignore pattern
					if p.shouldSkipPath(path) {
						continue
					}

					filesChan <- path
				}
			}
			return
		}

		// Walk the directory tree recursively
		err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Printf("Error accessing path %s: %v", path, err)
				return nil // Continue walking
			}

			// Skip directories based on ignore patterns
			if info.IsDir() {
				if p.shouldSkipPath(path) {
					return filepath.SkipDir
				}
				return nil
			}

			// Only process Go files
			if strings.HasSuffix(path, ".go") {
				// Skip test files if configured
				if p.ignoreTests && strings.HasSuffix(path, testSuffix) {
					return nil
				}

				// Skip files matching ignore pattern
				if p.shouldSkipPath(path) {
					return nil
				}

				// Send the file path to the channel
				filesChan <- path
			}

			return nil
		})

		if err != nil {
			log.Printf("Error walking directory tree: %v", err)
		}
	}()

	// Start file parser workers
	var parserWg sync.WaitGroup

	// Reuse FileSet in each worker
	fset := p.getFileSet()

	for i := 0; i < p.maxConcurrency; i++ {
		parserWg.Add(1)
		go func() {
			defer parserWg.Done()

			for filePath := range filesChan {
				// Parse a single file
				src, err := p.readFileEfficiently(filePath)
				if err != nil {
					log.Printf("Error reading file %s: %v", filePath, err)
					continue
				}

				f, err := parser.ParseFile(fset, filePath, src, 0)
				if err != nil {
					log.Printf("Error parsing file %s: %v", filePath, err)
					continue
				}

				// Process the file
				pkgName := f.Name.Name
				ast.Walk(&treeVisitor{
					fileSet:     fset,
					packageName: pkgName,
					fileName:    filePath,
					p:           p,
					ignoreRegex: p.ignoreStringsRegex,
				}, f)
			}
		}()
	}

	// Wait for all file collection to complete
	wg.Wait()
	// Wait for all file processing to complete
	parserWg.Wait()

	// Post-process and filter results
	p.ProcessResults()

	return p.strs, p.consts, nil
}

// parseTreeBatched implements batch processing for very large codebases.
// Instead of processing files immediately as they are found, it collects them
// in batches and processes each batch completely before moving to the next.
// This helps manage memory usage for extremely large codebases.
func (p *Parser) parseTreeBatched(rootPath string, recursive bool) (Strings, Constants, error) {
	var allFiles []string

	// First, collect all file paths that need to be processed
	if recursive {
		// If recursive, walk the entire directory tree
		err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Printf("Error accessing path %s: %v", path, err)
				return nil // Continue walking
			}

			// Only process Go files
			if !info.IsDir() && strings.HasSuffix(path, ".go") {
				// Skip test files if configured to do so
				if p.ignoreTests && strings.HasSuffix(path, testSuffix) {
					return nil
				}

				// Skip files matching ignore pattern
				if p.shouldSkipPath(path) {
					return nil
				}

				allFiles = append(allFiles, path)
			}

			return nil
		})

		if err != nil {
			return nil, nil, err
		}
	} else {
		// If not recursive, just read the files in the specified directory
		entries, err := os.ReadDir(rootPath)
		if err != nil {
			return nil, nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			path := filepath.Join(rootPath, entry.Name())

			// Only process Go files
			if strings.HasSuffix(path, ".go") {
				// Skip test files if configured to do so
				if p.ignoreTests && strings.HasSuffix(path, testSuffix) {
					continue
				}

				// Skip files matching ignore pattern
				if p.shouldSkipPath(path) {
					continue
				}

				allFiles = append(allFiles, path)
			}
		}
	}

	// Process files in batches
	totalFiles := len(allFiles)
	log.Printf("Found %d Go files to process in batches of %d", totalFiles, p.batchSize)

	for i := 0; i < totalFiles; i += p.batchSize {
		end := i + p.batchSize
		if end > totalFiles {
			end = totalFiles
		}

		batch := allFiles[i:end]
		log.Printf("Processing batch %d/%d (%d files)", (i/p.batchSize)+1, (totalFiles+p.batchSize-1)/p.batchSize, len(batch))

		// Process this batch concurrently
		var wg sync.WaitGroup
		fileChan := make(chan string, len(batch))

		// Start file processor workers
		for j := 0; j < p.maxConcurrency; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				fset := token.NewFileSet()

				for filePath := range fileChan {
					// Process each file
					src, err := p.readFileEfficiently(filePath)
					if err != nil {
						log.Printf("Error reading file %s: %v", filePath, err)
						continue
					}

					f, err := parser.ParseFile(fset, filePath, src, 0)
					if err != nil {
						log.Printf("Error parsing file %s: %v", filePath, err)
						continue
					}

					// Process the file
					pkgName := f.Name.Name
					ast.Walk(&treeVisitor{
						fileSet:     fset,
						packageName: pkgName,
						fileName:    filePath,
						p:           p,
						ignoreRegex: p.ignoreStringsRegex,
					}, f)
				}
			}()
		}

		// Queue all files in this batch
		for _, filePath := range batch {
			fileChan <- filePath
		}

		// Close the channel and wait for processing to complete
		close(fileChan)
		wg.Wait()

		// Optional: Run garbage collection between batches for very large codebases
		if totalFiles > 10000 && len(batch) >= 1000 {
			runtime.GC()
		}
	}

	// Post-process and filter results
	p.ProcessResults()

	return p.strs, p.consts, nil
}

// readFileEfficiently reads a file in the most efficient way.
// Benchmarks showed that for our specific use case, the standard
// library's ReadFile is already well-optimized.
func (p *Parser) readFileEfficiently(path string) ([]byte, error) {
	// Optimized file reading to reduce allocations
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			log.Printf("Error closing file: %v", closeErr)
		}
	}()

	// Get file size to allocate buffer exactly once
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	// For very small files, use ReadAll
	if info.Size() < 8192 {
		return io.ReadAll(f)
	}

	// For larger files, allocate exact buffer size to avoid resize allocations
	size := info.Size()
	buf := make([]byte, size)

	// Read in a single operation
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}

	return buf[:n], nil
}

// getFileSet returns a cached FileSet for reuse
func (p *Parser) getFileSet() *token.FileSet {
	p.fileSetMutex.Lock()
	defer p.fileSetMutex.Unlock()

	// Return existing cache if available
	if p.fileSetCache != nil {
		return p.fileSetCache
	}

	// Create a new one if needed
	p.fileSetCache = token.NewFileSet()
	return p.fileSetCache
}

// shouldSkipPath determines if a path should be skipped based on ignore patterns
func (p *Parser) shouldSkipPath(path string) bool {
	if p.ignoreRegex != nil {
		if p.ignoreRegex.MatchString(path) {
			return true
		}
	} else if len(p.ignore) != 0 {
		// Fallback to non-compiled regex if compilation failed
		match, err := regexp.MatchString(p.ignore, path)
		if err != nil {
			log.Printf("Error matching ignore pattern on %s: %v", path, err)
			return false
		}
		if match {
			return true
		}
	}
	return false
}

// IncrementStringCount safely increments the count for a string and returns the new count
func (p *Parser) IncrementStringCount(str string) int {
	p.stringCountMutex.Lock()
	defer p.stringCountMutex.Unlock()

	p.stringCount[str]++
	return p.stringCount[str]
}

// GetStringCount safely gets the count for a string
func (p *Parser) GetStringCount(str string) int {
	p.stringCountMutex.RLock()
	defer p.stringCountMutex.RUnlock()

	return p.stringCount[str]
}

// ProcessResults post-processes the raw results.
// It filters the discovered strings based on the parser's configuration:
// - Removes strings that don't meet the minimum occurrences threshold
// - Filters out strings matching the ignore pattern
// - Applies number range filtering if min/max values are set
func (p *Parser) ProcessResults() {
	p.stringMutex.Lock()
	defer p.stringMutex.Unlock()

	// Also acquire stringCount lock to ensure consistency during processing
	p.stringCountMutex.Lock()
	defer p.stringCountMutex.Unlock()

	for str := range p.strs {
		// Check count first as it's faster than looking at slice length
		count := p.stringCount[str]
		if count < p.minOccurrences {
			delete(p.strs, str)
			delete(p.stringCount, str)
			continue
		}

		// Apply ignoreStrings filter
		if p.ignoreStrings != "" {
			if p.ignoreStringsRegex != nil {
				// Use pre-compiled regex if available
				if p.ignoreStringsRegex.MatchString(str) {
					delete(p.strs, str)
					delete(p.stringCount, str)
					continue
				}
			} else {
				// Fallback to the non-compiled version
				match, err := regexp.MatchString(p.ignoreStrings, str)
				if err != nil {
					log.Println(err)
				}
				if match {
					delete(p.strs, str)
					delete(p.stringCount, str)
					continue
				}
			}
		}

		// Apply number range filtering if applicable
		if i, err := strconv.ParseInt(str, 0, 0); err == nil {
			if (p.numberMin != 0 && i < int64(p.numberMin)) ||
				(p.numberMax != 0 && i > int64(p.numberMax)) {
				delete(p.strs, str)
				delete(p.stringCount, str)
			}
		}
	}
}

// Strings maps string literals to their positions in the code.
type Strings map[string][]ExtendedPos

// Constants maps string values to their constant definitions.
type Constants map[string]ConstType

// ConstType holds information about a constant declaration.
type ConstType struct {
	// Using embedded Position to save memory vs. a separate field
	token.Position
	// Interned strings to reduce memory usage
	Name        string
	packageName string
}

// ExtendedPos extends token.Position with package information.
// This structure is optimized for memory usage in large codebases.
type ExtendedPos struct {
	// Using embedded Position to save memory vs. a separate field
	token.Position
	// Interned package name to reduce memory usage when many positions
	// reference the same package
	packageName string
}

// Type represents the context in which a string literal appears.
type Type int

const (
	// Assignment represents a string in an assignment context (e.g., x := "foo")
	Assignment Type = iota
	// Binary represents a string in a binary expression (e.g., x == "foo")
	Binary
	// Case represents a string in a case clause (e.g., case "foo":)
	Case
	// Return represents a string in a return statement (e.g., return "foo")
	Return
	// Call represents a string passed as an argument to a function call (e.g., f("foo"))
	Call
)
