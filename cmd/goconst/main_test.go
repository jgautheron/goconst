package main

import (
	"bytes"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgautheron/goconst"
)

func TestUsage(t *testing.T) {
	var buf bytes.Buffer
	usage(&buf)
	output := buf.String()

	// Check that usage output contains expected sections
	expectedSections := []string{
		"goconst: find repeated strings that could be replaced by a constant",
		"Usage:",
		"Flags:",
		"Examples:",
	}

	for _, section := range expectedSections {
		if !strings.Contains(output, section) {
			t.Errorf("Expected usage output to contain %q", section)
		}
	}

	// Check that all flags are documented
	expectedFlags := []string{
		"-ignore",
		"-ignore-strings",
		"-ignore-tests",
		"-min-occurrences",
		"-min-length",
		"-match-constant",
		"-numbers",
		"-min",
		"-max",
		"-output",
		"-set-exit-status",
	}

	for _, flag := range expectedFlags {
		if !strings.Contains(output, flag) {
			t.Errorf("Expected usage output to document flag %q", flag)
		}
	}
}

func TestRun(t *testing.T) {
	// Create a temporary directory with a test file
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
}`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Run the CLI run function
	hasIssues, err := run(tempDir)
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if !hasIssues {
		t.Error("run() returned false, expected true")
	}
}

func TestInvalidOutputFormat(t *testing.T) {
	// Create a minimal temp file just to have something to analyze
	tempDir, err := os.MkdirTemp("", "goconst-output-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	testFile := filepath.Join(tempDir, "simple.go")
	testContent := `package test
func test() {
	a := "test"
	b := "test"
}`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Save original flags
	oldOutput := *flagOutput

	// Set invalid output format
	*flagOutput = "invalid"

	// Restore when done
	defer func() {
		*flagOutput = oldOutput
	}()

	// Should return error when run
	_, err = run(tempDir)
	if err == nil {
		t.Error("Expected error with invalid output format")
	}
}

func TestOutputFormatting(t *testing.T) {
	// Create a file with duplicates
	tempDir, err := os.MkdirTemp("", "goconst-format-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	testFile := filepath.Join(tempDir, "format.go")
	testContent := `package test
const TestConst = "should_be_constant"
func test() {
	// These should be detected
	a := "should_be_constant"
	b := "should_be_constant"
	
	// This should match the constant
	c := "should_be_constant"
}`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Test text output format
	t.Run("text output", func(t *testing.T) {
		// Save original stdout and flags
		oldStdout := os.Stdout
		oldOutput := *flagOutput
		oldMatchConstant := *flagMatchConstant

		// Set flags
		*flagOutput = "text"
		*flagMatchConstant = true

		// Redirect stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Restore when done
		defer func() {
			os.Stdout = oldStdout
			*flagOutput = oldOutput
			*flagMatchConstant = oldMatchConstant
		}()

		// Run analysis
		hasIssues, err := run(tempDir)
		if err := w.Close(); err != nil {
			t.Errorf("Failed to close writer: %v", err)
		}
		out, _ := io.ReadAll(r)
		output := string(out)

		if err != nil {
			t.Errorf("run() error = %v", err)
		}
		if !hasIssues {
			t.Error("run() returned false, expected true")
		}

		// Check for expected output patterns
		expectedPatterns := []string{
			"should_be_constant",
			"occurrence",
			"TestConst",
		}

		for _, pattern := range expectedPatterns {
			if !strings.Contains(output, pattern) {
				t.Errorf("Output missing expected pattern: %q", pattern)
			}
		}
	})

	// Test JSON output format
	t.Run("json output", func(t *testing.T) {
		// Save original stdout and flags
		oldStdout := os.Stdout
		oldOutput := *flagOutput

		// Set flags
		*flagOutput = "json"

		// Redirect stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Restore when done
		defer func() {
			os.Stdout = oldStdout
			*flagOutput = oldOutput
		}()

		// Run analysis
		hasIssues, err := run(tempDir)
		if err := w.Close(); err != nil {
			t.Errorf("Failed to close writer: %v", err)
		}
		out, _ := io.ReadAll(r)
		output := string(out)

		if err != nil {
			t.Errorf("run() error = %v", err)
		}
		if !hasIssues {
			t.Error("run() returned false, expected true")
		}

		// Check for expected JSON elements
		expectedPatterns := []string{
			`"strings"`,
			`"should_be_constant"`,
			`"constants"`,
		}

		for _, pattern := range expectedPatterns {
			if !strings.Contains(output, pattern) {
				t.Errorf("JSON output missing expected pattern: %q", pattern)
			}
		}
	})
}

func TestGroupedOutput(t *testing.T) {
	// Create a file with duplicates
	tempDir, err := os.MkdirTemp("", "goconst-grouped-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Errorf("Failed to remove temp directory: %v", err)
		}
	}()

	testFile := filepath.Join(tempDir, "grouped.go")
	testContent := `package test
func test() {
	a := "duplicate1"
	b := "duplicate1"
	c := "duplicate2"
	d := "duplicate2"
}`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Test with grouped flag
	t.Run("grouped output", func(t *testing.T) {
		// Save original stdout and flags
		oldStdout := os.Stdout
		oldOutput := *flagOutput
		oldGrouped := *flagGrouped

		// Set flags
		*flagOutput = "text"
		*flagGrouped = true

		// Redirect stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		// Restore when done
		defer func() {
			os.Stdout = oldStdout
			*flagOutput = oldOutput
			*flagGrouped = oldGrouped
		}()

		// Run analysis
		_, err := run(tempDir)
		if err := w.Close(); err != nil {
			t.Errorf("Failed to close writer: %v", err)
		}
		out, _ := io.ReadAll(r)
		output := string(out)

		if err != nil {
			t.Errorf("run() error = %v", err)
		}

		// Count occurrences of the strings in output
		// With grouped=true, each duplicate string should appear only once
		duplicate1Count := strings.Count(output, "duplicate1")
		duplicate2Count := strings.Count(output, "duplicate2")

		if duplicate1Count > 1 {
			t.Errorf("Grouped output shows 'duplicate1' %d times, expected 1", duplicate1Count)
		}

		if duplicate2Count > 1 {
			t.Errorf("Grouped output shows 'duplicate2' %d times, expected 1", duplicate2Count)
		}
	})
}

func TestParseCommaSeparatedValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{name: "empty", input: "", expected: nil},
		{name: "single value", input: "foo", expected: []string{"foo"}},
		{name: "simple split", input: "a,b,c", expected: []string{"a", "b", "c"}},
		{name: "trailing empty", input: "a,b,", expected: []string{"a", "b", ""}},
		{name: "leading empty", input: ",a", expected: []string{"", "a"}},
		{name: "empty between", input: "a,,b", expected: []string{"a", "", "b"}},
		{name: "escaped comma", input: `foo\,bar`, expected: []string{"foo,bar"}},
		{name: "multiple escaped commas", input: `a\,b\,c`, expected: []string{"a,b,c"}},
		{name: "mixed escaped and normal", input: `a,b\,c,d`, expected: []string{"a", "b,c", "d"}},
		{name: "trailing backslash no escaped comma", input: `foo\`, expected: []string{`foo\`}},
		{name: "escaped path trailing backslash", input: `a\,b\`, expected: []string{"a,b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaSeparatedValues(tt.input)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseCommaSeparatedValues(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestOccurrences(t *testing.T) {
	pos := func(file string, line, col int) goconst.ExtendedPos {
		return goconst.ExtendedPos{
			Position: token.Position{Filename: file, Line: line, Column: col},
		}
	}

	tests := []struct {
		name    string
		item    []goconst.ExtendedPos
		current goconst.ExtendedPos
		want    string
	}{
		{
			name:    "single item equals current",
			item:    []goconst.ExtendedPos{pos("a.go", 1, 1)},
			current: pos("a.go", 1, 1),
			want:    "",
		},
		{
			name:    "two items current is first",
			item:    []goconst.ExtendedPos{pos("a.go", 1, 1), pos("b.go", 2, 3)},
			current: pos("a.go", 1, 1),
			want:    "b.go:2:3",
		},
		{
			name:    "three items current is middle",
			item:    []goconst.ExtendedPos{pos("a.go", 1, 1), pos("b.go", 2, 3), pos("c.go", 4, 5)},
			current: pos("b.go", 2, 3),
			want:    "a.go:1:1 c.go:4:5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := occurrences(tt.item, tt.current)
			if got != tt.want {
				t.Errorf("occurrences() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintOutput_EmptyMaps(t *testing.T) {
	t.Run("text empty", func(t *testing.T) {
		hasIssues, err := printOutput(goconst.Strings{}, goconst.Constants{}, "text")
		if err != nil {
			t.Fatalf("printOutput() error = %v", err)
		}
		if hasIssues {
			t.Error("printOutput() returned true, want false")
		}
	})

	t.Run("json nil", func(t *testing.T) {
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()

		hasIssues, err := printOutput(nil, nil, "json")
		w.Close()
		io.ReadAll(r)

		if err != nil {
			t.Fatalf("printOutput() error = %v", err)
		}
		if hasIssues {
			t.Error("printOutput() returned true, want false")
		}
	})
}
