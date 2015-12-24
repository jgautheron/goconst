package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const usageDoc = `goconst: find repeated strings that could be replaced by a constant

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
`

var (
	flagIgnore         = flag.String("ignore", "", "ignore files matching the given regular expression")
	flagIgnoreTests    = flag.Bool("ignore-tests", true, "exclude tests from the search")
	flagMinOccurrences = flag.Int("min-occurrences", 2, "report from how many occurrences")
	flagMatchConstant  = flag.Bool("match-constant", false, "look for existing constants matching the strings")
	flagOutput         = flag.String("output", "text", "output formatting")

	sourcePath = ""
	strs       = map[string][]extendedPos{}
	consts     = map[string]constType{}
)

type constType struct {
	token.Position
	name, packageName string
}

type extendedPos struct {
	token.Position
	packageName string
}

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
	}
	flag.Parse()
	log.SetPrefix("goconst: ")

	args := flag.Args()
	if len(args) != 1 {
		usage()
	}
	sourcePath = args[0]

	if err := parseTree(); err != nil {
		log.Println(err)
		os.Exit(1)
	}

	printOutput()
}

func usage() {
	fmt.Fprintf(os.Stderr, usageDoc)
	os.Exit(1)
}

func parseTree() error {
	pathLen := len(sourcePath)
	// Parse recursively the given path if the recursive notation is found
	if pathLen >= 5 && sourcePath[pathLen-3:] == "..." {
		filepath.Walk(sourcePath[:pathLen-3], func(p string, f os.FileInfo, err error) error {
			if err != nil {
				log.Println(err)
				// resume walking
				return nil
			}

			if f.IsDir() {
				parseDir(p)
			}
			return nil
		})
	} else {
		parseDir(sourcePath)
	}
	return nil
}

func parseDir(dir string) error {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		valid, name := true, info.Name()

		if *flagIgnoreTests {
			if strings.HasSuffix(name, "_test.go") {
				valid = false
			}
		}

		if len(*flagIgnore) != 0 {
			match, err := regexp.MatchString(*flagIgnore, dir+name)
			if err != nil {
				log.Fatal(err)
				return true
			}
			if match {
				valid = false
			}
		}

		return valid
	}, 0)
	if err != nil {
		return err
	}

	for _, pkg := range pkgs {
		for fn, f := range pkg.Files {
			ast.Walk(&TreeVisitor{
				fileSet:     fset,
				packageName: pkg.Name,
				fileName:    fn,
			}, f)
		}
	}

	return nil
}

func printOutput() {
	// Filter out items whose occurrences don't match the min value
	for str, item := range strs {
		if len(item) < *flagMinOccurrences {
			delete(strs, str)
		}
	}

	switch *flagOutput {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.Encode(struct {
			Strings   map[string][]extendedPos
			Constants map[string]constType `json:",omitEmpty"`
		}{
			strs, consts,
		})
	case "text":
		for str, item := range strs {
			fmt.Printf(`%d occurrences of "%s" found:`, len(item), str)
			for _, xpos := range item {
				fmt.Printf("\n\t%s", xpos.String())
			}
			fmt.Print("\n")

			if !*flagMatchConstant {
				continue
			}
			if cst, ok := consts[str]; ok {
				// const should be in the same package and exported
				fmt.Printf(`A matching constant has been found for "%s": %s`, str, cst.name)
				fmt.Printf("\n\t%s\n", cst.String())
			}
		}
	}
}

// TreeVisitor carries the package name and file name
// for passing it to the imports map, and the fileSet for
// retrieving the token.Position.
type TreeVisitor struct {
	fileSet               *token.FileSet
	packageName, fileName string
}

// Visit browses the AST tree for strings that could be potentially
// replaced by constants.
// A map of existing constants is built as well (-match-constant).
func (v *TreeVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return v
	}

	// A single case with "ast.BasicLit" would be much easier
	// but then we wouldn't be able to tell in which context
	// the string is defined (could be a constant definition).
	switch t := node.(type) {
	// Scan for constants in an attempt to match strings with existing constants
	case *ast.GenDecl:
		if !*flagMatchConstant {
			return v
		}
		if t.Tok != token.CONST {
			return v
		}

		for _, spec := range t.Specs {
			val := spec.(*ast.ValueSpec)
			for i, str := range val.Values {
				lit, ok := str.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}

				v.addConst(val.Names[i].Name, lit.Value, val.Names[i].Pos())
			}
		}

		// foo := "moo"
	case *ast.AssignStmt:
		for _, rhs := range t.Rhs {
			lit, ok := rhs.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}

			v.addString(lit.Value, rhs.(*ast.BasicLit).Pos())
		}

	// if foo == "moo"
	case *ast.BinaryExpr:
		if t.Op != token.EQL && t.Op != token.NEQ {
			return v
		}

		var lit *ast.BasicLit
		var ok bool

		lit, ok = t.X.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING {
			v.addString(lit.Value, lit.Pos())
		}

		lit, ok = t.Y.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING {
			v.addString(lit.Value, lit.Pos())
		}

	// case "foo":
	case *ast.CaseClause:
		for _, item := range t.List {
			lit, ok := item.(*ast.BasicLit)
			if ok && lit.Kind == token.STRING {
				v.addString(lit.Value, lit.Pos())
			}
		}

	// return "boo"
	case *ast.ReturnStmt:
		for _, item := range t.Results {
			lit, ok := item.(*ast.BasicLit)
			if ok && lit.Kind == token.STRING {
				v.addString(lit.Value, lit.Pos())
			}
		}
	}

	return v
}

// addString adds a string in the map along with its position in the tree.
func (v *TreeVisitor) addString(str string, pos token.Pos) {
	str = strings.Replace(str, `"`, "", 2)

	// Ignore empty strings
	if len(str) == 0 {
		return
	}

	_, ok := strs[str]
	if !ok {
		strs[str] = make([]extendedPos, 0)
	}
	strs[str] = append(strs[str], extendedPos{
		packageName: v.packageName,
		Position:    v.fileSet.Position(pos),
	})
}

// addConst adds a const in the map along with its position in the tree.
func (v *TreeVisitor) addConst(name string, val string, pos token.Pos) {
	val = strings.Replace(val, `"`, "", 2)
	consts[val] = constType{
		name:        name,
		packageName: v.packageName,
		Position:    v.fileSet.Position(pos),
	}
}
