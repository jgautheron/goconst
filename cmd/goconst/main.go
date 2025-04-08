package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/jgautheron/goconst"
)

const usageDoc = `goconst: find repeated strings that could be replaced by a constant

Usage:

  goconst ARGS <directory> [<directory>...]

Flags:

  -ignore            exclude files matching the given regular expression
  -ignore-strings    exclude strings matching the given regular expression
  -ignore-tests      exclude tests from the search (default: true)
  -min-occurrences   report from how many occurrences (default: 2)
  -min-length        only report strings with the minimum given length (default: 3)
  -match-constant    look for existing constants matching the strings
  -find-duplicates   look for constants with identical values
  -eval-const-expr   enable evaluation of constant expressions (e.g., Prefix + "suffix")
  -numbers           search also for duplicated numbers
  -min               minimum value, only works with -numbers
  -max               maximum value, only works with -numbers
  -output            output formatting (text or json)
  -set-exit-status   Set exit status to 2 if any issues are found
  -grouped           print single line per match, only works with -output text

Examples:

  goconst ./...
  goconst -ignore "yacc|\.pb\." $GOPATH/src/github.com/cockroachdb/cockroach/...
  goconst -min-occurrences 3 -output json $GOPATH/src/github.com/cockroachdb/cockroach
  goconst -numbers -min 60 -max 512 .
  goconst -min-occurrences 5 $(go list -m -f '{{.Dir}}')
  goconst -eval-const-expr -match-constant . # Matches constant expressions like Prefix + "suffix"
`

var (
	flagIgnore         = flag.String("ignore", "", "ignore files matching the given regular expression")
	flagIgnoreStrings  = flag.String("ignore-strings", "", "ignore strings matching the given regular expressions (comma separated)")
	flagIgnoreTests    = flag.Bool("ignore-tests", true, "exclude tests from the search")
	flagMinOccurrences = flag.Int("min-occurrences", 2, "report from how many occurrences")
	flagMinLength      = flag.Int("min-length", 3, "only report strings with the minimum given length")
	flagMatchConstant  = flag.Bool("match-constant", false, "look for existing constants matching the strings")
	flagFindDuplicates = flag.Bool("find-duplicates", false, "look for constants with duplicated values")
	flagEvalConstExpr  = flag.Bool("eval-const-expr", false, "enable evaluation of constant expressions (e.g., Prefix + \"suffix\")")
	flagNumbers        = flag.Bool("numbers", false, "search also for duplicated numbers")
	flagMin            = flag.Int("min", 0, "minimum value, only works with -numbers")
	flagMax            = flag.Int("max", 0, "maximum value, only works with -numbers")
	flagOutput         = flag.String("output", "text", "output formatting")
	flagSetExitStatus  = flag.Bool("set-exit-status", false, "Set exit status to 2 if any issues are found")
	flagGrouped        = flag.Bool("grouped", false, "print single line per match, only works with -output text")
)

func main() {
	flag.Usage = func() {
		usage(os.Stderr)
	}
	flag.Parse()
	log.SetPrefix("goconst: ")

	args := flag.Args()
	if len(args) < 1 {
		usage(os.Stderr)
		os.Exit(1)
	}

	lintFailed := false
	for _, path := range args {
		anyIssues, err := run(path)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}

		if anyIssues {
			lintFailed = true
		}
	}

	if lintFailed && *flagSetExitStatus {
		os.Exit(2)
	}
}

// run analyzes a single path for repeated strings that could be constants.
// It returns true if any issues were found, and an error if the analysis failed.
func run(path string) (bool, error) {
	// Parse ignore strings - handling comma-separated values
	var ignoreStrings []string
	if *flagIgnoreStrings != "" {
		// Split by commas but handle escaping
		ignoreStrings = parseCommaSeparatedValues(*flagIgnoreStrings)
	}

	gco := goconst.NewWithIgnorePatterns(
		path,
		*flagIgnore,
		ignoreStrings,
		*flagIgnoreTests,
		*flagMatchConstant,
		*flagNumbers,
		*flagFindDuplicates,
		*flagEvalConstExpr,
		*flagMin,
		*flagMax,
		*flagMinLength,
		*flagMinOccurrences,
		map[goconst.Type]bool{},
	)
	strs, consts, err := gco.ParseTree()
	if err != nil {
		return false, err
	}

	return printOutput(strs, consts, *flagOutput)
}

// parseCommaSeparatedValues splits a comma-separated string into a slice of strings,
// handling escaping of commas within values.
func parseCommaSeparatedValues(input string) []string {
	if input == "" {
		return nil
	}

	// Simple case - no escaping needed
	if !strings.Contains(input, "\\,") {
		return strings.Split(input, ",")
	}

	// Handle escaped commas
	var result []string
	var current strings.Builder
	escaped := false

	for _, char := range input {
		if escaped {
			if char == ',' {
				current.WriteRune(',')
			} else {
				current.WriteRune('\\')
				current.WriteRune(char)
			}
			escaped = false
		} else if char == '\\' {
			escaped = true
		} else if char == ',' {
			result = append(result, current.String())
			current.Reset()
		} else {
			current.WriteRune(char)
		}
	}

	// Don't forget the last value
	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// usage prints the usage documentation to the specified writer.
func usage(out io.Writer) {
	if _, err := fmt.Fprint(out, usageDoc); err != nil {
		log.Printf("Error writing usage doc: %v", err)
	}
}

// printOutput formats and displays the analysis results based on the specified output format.
// It returns true if any issues were found, and an error if output formatting failed.
func printOutput(strs goconst.Strings, consts goconst.Constants, output string) (bool, error) {
	switch output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		err := enc.Encode(struct {
			Strings   goconst.Strings   `json:"strings"`
			Constants goconst.Constants `json:"constants"`
		}{
			Strings:   strs,
			Constants: consts,
		})
		if err != nil {
			return false, err
		}
	case "text":
		for str, item := range strs {
			for _, xpos := range item {
				fmt.Printf(
					`%s:%d:%d:%d other occurrence(s) of %q found in: %s`,
					xpos.Filename,
					xpos.Line,
					xpos.Column,
					len(item)-1,
					str,
					occurrences(item, xpos),
				)
				fmt.Print("\n")

				if *flagGrouped {
					break
				}
			}

			if len(consts) == 0 {
				continue
			}
			if csts, ok := consts[str]; ok && len(csts) > 0 {
				// const should be in the same package and exported
				fmt.Printf(`A matching constant has been found for %q: %s`, str, csts[0].Name)
				fmt.Printf("\n\t%s\n", csts[0].String())
			}
		}
		for val, csts := range consts {
			if len(csts) > 1 {
				fmt.Printf("Duplicate constant(s) with value %q have been found:\n", val)

				for i := 0; i < len(csts); i++ {
					fmt.Printf("\t%s: %s\n", csts[i].String(), csts[i].Name)
				}
			}
		}
	default:
		return false, fmt.Errorf("unsupported output format: %s", output)
	}
	return len(strs)+len(consts) > 0, nil
}

// occurrences formats a list of all occurrences of a string, excluding the current position.
func occurrences(item []goconst.ExtendedPos, current goconst.ExtendedPos) string {
	occurrences := []string{}
	for _, xpos := range item {
		if xpos == current {
			continue
		}
		occurrences = append(occurrences, fmt.Sprintf(
			"%s:%d:%d", xpos.Filename, xpos.Line, xpos.Column,
		))
	}
	return strings.Join(occurrences, " ")
}
