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
`

var (
	flagIgnore         = flag.String("ignore", "", "ignore files matching the given regular expression")
	flagIgnoreStrings  = flag.String("ignore-strings", "", "ignore strings matching the given regular expression")
	flagIgnoreTests    = flag.Bool("ignore-tests", true, "exclude tests from the search")
	flagMinOccurrences = flag.Int("min-occurrences", 2, "report from how many occurrences")
	flagMinLength      = flag.Int("min-length", 3, "only report strings with the minimum given length")
	flagMatchConstant  = flag.Bool("match-constant", false, "look for existing constants matching the strings")
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

func run(path string) (bool, error) {
	gco := goconst.New(
		path,
		*flagIgnore,
		*flagIgnoreStrings,
		*flagIgnoreTests,
		*flagMatchConstant,
		*flagNumbers,
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

func usage(out io.Writer) {
	fmt.Fprintf(out, usageDoc)
}

func printOutput(strs goconst.Strings, consts goconst.Constants, output string) (bool, error) {
	switch output {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		err := enc.Encode(struct {
			Strings   goconst.Strings   `json:"strings,omitEmpty"`
			Constants goconst.Constants `json:"constants,omitEmpty"`
		}{
			strs, consts,
		})
		if err != nil {
			return false, err
		}
	case "text":
		for str, item := range strs {
			for _, xpos := range item {
				fmt.Printf(
					`%s:%d:%d:%d other occurrence(s) of "%s" found in: %s`,
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
			if cst, ok := consts[str]; ok {
				// const should be in the same package and exported
				fmt.Printf(`A matching constant has been found for "%s": %s`, str, cst.Name)
				fmt.Printf("\n\t%s\n", cst.String())
			}
		}
	default:
		return false, fmt.Errorf(`Unsupported output format: %s`, output)
	}
	return len(strs)+len(consts) > 0, nil
}

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
