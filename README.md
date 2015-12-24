# goconst

Find repeated strings that could be replaced by a constant.

### Get Started

    $ go get github.com/jgautheron/goconst
    $ goconst -path $GOPATH/src/github.com/cockroachdb/cockroach/...

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
