# goconst

Find repeated strings that could be replaced by a constant.

### Motivation

There are obvious benefits to using constants instead of repeating strings, mostly to ease maintenance. Cannot argue against changing a single constant versus many strings.

While this could be considered a beginner mistake, across time, multiple packages and large codebases, some repetition could have slipped in.

### Get Started

    $ go get github.com/jgautheron/goconst
    $ goconst ./...

### Usage

```
Usage:

  goconst ARGS <directory>

Flags:

  -ignore            exclude files matching the given regular expression
  -ignore-tests      exclude tests from the search (default: true)
  -min-occurrences   report from how many occurrences (default: 2)
  -match-constant    look for existing constants matching the strings
  -output            output formatting (text or json)

Examples:

  goconst ./...
  goconst -ignore "yacc|\.pb\." $GOPATH/src/github.com/cockroachdb/cockroach/...
  goconst -min-occurrences 3 -output json $GOPATH/src/github.com/cockroachdb/cockroach
```

### License
MIT
