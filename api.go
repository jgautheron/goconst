package goconst

import (
	"go/ast"
	"go/token"
	"strings"
	"sync"
)

// Issue represents a finding of duplicated strings or numbers.
// Each Issue includes the position where it was found, how many times it occurs,
// the string itself, and any matching constant name.
type Issue struct {
	Pos              token.Position
	OccurrencesCount int
	Str              string
	MatchingConst    string
}

// IssuePool provides a pool of Issue slices to reduce allocations
var IssuePool = sync.Pool{
	New: func() interface{} {
		return make([]Issue, 0, 100)
	},
}

// GetIssueBuffer retrieves an Issue slice from the pool
func GetIssueBuffer() []Issue {
	return IssuePool.Get().([]Issue)[:0] // Reset length but keep capacity
}

// PutIssueBuffer returns an Issue slice to the pool
func PutIssueBuffer(issues []Issue) {
	// Make sure to clear references before returning to pool
	for i := range issues {
		issues[i].MatchingConst = ""
		issues[i].Str = ""
	}
	// Return the slice to the pool
	issuesCopy := make([]Issue, 0, cap(issues))
	IssuePool.Put(&issuesCopy)
}

// Config contains all configuration options for the goconst analyzer.
type Config struct {
	// IgnoreStrings is a regular expression to filter strings
	IgnoreStrings string
	// IgnoreTests indicates whether test files should be excluded
	IgnoreTests bool
	// MatchWithConstants enables matching strings with existing constants
	MatchWithConstants bool
	// MinStringLength is the minimum length a string must have to be reported
	MinStringLength int
	// MinOccurrences is the minimum number of occurrences required to report a string
	MinOccurrences int
	// ParseNumbers enables detection of duplicated numbers
	ParseNumbers bool
	// NumberMin sets the minimum value for reported number matches
	NumberMin int
	// NumberMax sets the maximum value for reported number matches
	NumberMax int
	// ExcludeTypes allows excluding specific types of contexts
	ExcludeTypes map[Type]bool
}

// Run analyzes the provided AST files for duplicated strings or numbers
// according to the provided configuration.
// It returns a slice of Issue objects containing the findings.
func Run(files []*ast.File, fset *token.FileSet, cfg *Config) ([]Issue, error) {
	p := New(
		"",
		"",
		cfg.IgnoreStrings,
		cfg.IgnoreTests,
		cfg.MatchWithConstants,
		cfg.ParseNumbers,
		cfg.NumberMin,
		cfg.NumberMax,
		cfg.MinStringLength,
		cfg.MinOccurrences,
		cfg.ExcludeTypes,
	)

	// Pre-allocate slice based on estimated result size
	expectedIssues := len(files) * 5 // Assuming average of 5 issues per file
	if expectedIssues > 1000 {
		expectedIssues = 1000 // Cap at reasonable maximum
	}

	// Get issue buffer from pool instead of allocating
	issueBuffer := GetIssueBuffer()
	if cap(issueBuffer) < expectedIssues {
		// Only allocate new buffer if existing one is too small
		PutIssueBuffer(issueBuffer)
		issueBuffer = make([]Issue, 0, expectedIssues)
	}

	// Process files concurrently
	var wg sync.WaitGroup
	sem := make(chan struct{}, p.maxConcurrency)

	// Create a filtered files slice with capacity hint
	filteredFiles := make([]*ast.File, 0, len(files))

	// Filter test files first if needed
	for _, f := range files {
		if p.ignoreTests {
			if filename := fset.Position(f.Pos()).Filename; strings.HasSuffix(filename, "_test.go") {
				continue
			}
		}
		filteredFiles = append(filteredFiles, f)
	}

	// Process each file in parallel
	for _, f := range filteredFiles {
		wg.Add(1)
		sem <- struct{}{} // acquire semaphore

		go func(f *ast.File) {
			defer func() {
				<-sem // release semaphore
				wg.Done()
			}()

			// Use empty interned strings for package/file names
			// The visitor logic will set these appropriately
			emptyStr := InternString("")

			ast.Walk(&treeVisitor{
				fileSet:     fset,
				packageName: emptyStr,
				fileName:    emptyStr,
				p:           p,
				ignoreRegex: p.ignoreStringsRegex,
			}, f)
		}(f)
	}

	wg.Wait()

	p.ProcessResults()

	// Process each string that passed the filters
	p.stringMutex.RLock()
	p.stringCountMutex.RLock()

	// Get a string buffer from pool instead of allocating
	stringKeys := GetStringBuffer()

	// Create an array of strings to sort for stable output
	for str := range p.strs {
		if count := p.stringCount[str]; count >= p.minOccurrences {
			stringKeys = append(stringKeys, str)
		}
	}

	// Process strings in a predictable order for stable output
	for _, str := range stringKeys {
		positions := p.strs[str]
		if len(positions) == 0 {
			continue
		}

		// Use the first position as representative
		fi := positions[0]

		// Create issue using the counted value to avoid recounting
		issue := Issue{
			Pos:              fi.Position,
			OccurrencesCount: p.stringCount[str],
			Str:              str,
		}

		// Check for matching constants
		if len(p.consts) > 0 {
			p.constMutex.RLock()
			if cst, ok := p.consts[str]; ok {
				// const should be in the same package and exported
				issue.MatchingConst = cst.Name
			}
			p.constMutex.RUnlock()
		}

		issueBuffer = append(issueBuffer, issue)
	}

	p.stringCountMutex.RUnlock()
	p.stringMutex.RUnlock()

	// Return string buffer to pool
	PutStringBuffer(stringKeys)

	// Don't return the buffer to pool as the caller now owns it
	return issueBuffer, nil
}
