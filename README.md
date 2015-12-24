# goconst

Find repeated strings that could be replaced by a constant.

### Get Started

    $ go get github.com/jgautheron/goconst
    $ goconst -path $GOPATH/src/github.com/cockroachdb/cockroach/...

### Usage

```
Usage:

  goconst -path <directory>

Flags:

  -path              path to be scanned for imports
  -ignore            exclude files matching the given regular expression
  -ignore-tests      exclude tests from the search (default: true)
  -min-occurrences   report from how many occurrences (default: 2)
  -match-constant    look for existing constants matching the strings

Examples:

  goconst -path $GOPATH/src/github.com/cockroachdb/cockroach/... -ignore "sql|rpc"
  goconst -path $GOPATH/src/github.com/cockroachdb/cockroach -min-occurrences 3 -output json
```
