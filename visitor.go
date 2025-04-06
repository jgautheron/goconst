package goconst

import (
	"go/ast"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// treeVisitor is used to walk the AST and find strings that could be constants.
type treeVisitor struct {
	fileSet     *token.FileSet
	packageName string
	fileName    string
	p          *Parser
	ignoreRegex *regexp.Regexp
}

// Visit browses the AST tree for strings that could be potentially
// replaced by constants.
// A map of existing constants is built as well (-match-constant).
func (v *treeVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return v
	}

	// A single case with "ast.BasicLit" would be much easier
	// but then we wouldn't be able to tell in which context
	// the string is defined (could be a constant definition).
	switch t := node.(type) {
	// Scan for constants in an attempt to match strings with existing constants
	case *ast.GenDecl:
		if !v.p.matchConstant && !v.p.findDuplicates {
			return v
		}
		if t.Tok != token.CONST {
			return v
		}

		for _, spec := range t.Specs {
			val := spec.(*ast.ValueSpec)
			for i, expr := range val.Values {
				// Handle basic literals (existing code)
				if lit, ok := expr.(*ast.BasicLit); ok && v.isSupported(lit.Kind) {
					v.addConst(val.Names[i].Name, lit.Value, val.Names[i].Pos())
					continue
				}
				
				// Handle constant expressions
				if v.p.evalConstExpressions {
					// Try to evaluate constant expressions using Go's evaluator
					if strValue := v.evaluateConstExpr(expr); strValue != "" {
						v.addConstWithValue(val.Names[i].Name, strValue, val.Names[i].Pos())
					}
				}
			}
		}

	// foo := "moo"
	case *ast.AssignStmt:
		for _, rhs := range t.Rhs {
			lit, ok := rhs.(*ast.BasicLit)
			if !ok || !v.isSupported(lit.Kind) {
				continue
			}

			v.addString(lit.Value, rhs.(*ast.BasicLit).Pos(), Assignment)
		}

	// if foo == "moo"
	case *ast.BinaryExpr:
		if t.Op != token.EQL && t.Op != token.NEQ {
			return v
		}

		var lit *ast.BasicLit
		var ok bool

		lit, ok = t.X.(*ast.BasicLit)
		if ok && v.isSupported(lit.Kind) {
			v.addString(lit.Value, lit.Pos(), Binary)
		}

		lit, ok = t.Y.(*ast.BasicLit)
		if ok && v.isSupported(lit.Kind) {
			v.addString(lit.Value, lit.Pos(), Binary)
		}

	// case "foo":
	case *ast.CaseClause:
		for _, item := range t.List {
			lit, ok := item.(*ast.BasicLit)
			if ok && v.isSupported(lit.Kind) {
				v.addString(lit.Value, lit.Pos(), Case)
			}
		}

	// return "boo"
	case *ast.ReturnStmt:
		for _, item := range t.Results {
			lit, ok := item.(*ast.BasicLit)
			if ok && v.isSupported(lit.Kind) {
				v.addString(lit.Value, lit.Pos(), Return)
			}
		}

	// fn("http://")
	case *ast.CallExpr:
		for _, item := range t.Args {
			lit, ok := item.(*ast.BasicLit)
			if ok && v.isSupported(lit.Kind) {
				v.addString(lit.Value, lit.Pos(), Call)
			}
		}
	}

	return v
}

// addString adds a string in the map along with its position in the tree.
func (v *treeVisitor) addString(str string, pos token.Pos, typ Type) {
	// Early type exclusion check
	ok, excluded := v.p.excludeTypes[typ]
	if ok && excluded {
		return
	}

	// Drop quotes if any
	var unquotedStr string
	if strings.HasPrefix(str, `"`) || strings.HasPrefix(str, "`") {
		var err error
		// Reuse strings from pool if possible to avoid allocations
		sb := GetStringBuilder()
		defer PutStringBuilder(sb)

		unquotedStr, err = strconv.Unquote(str)
		if err != nil {
			// If unquoting fails, manually strip quotes
			// This avoids additional temporary strings
			if len(str) >= 2 {
				sb.WriteString(str[1 : len(str)-1])
				unquotedStr = sb.String()
			} else {
				unquotedStr = str
			}
		}
	} else {
		unquotedStr = str
	}

	// Early length check
	if len(unquotedStr) == 0 || len(unquotedStr) < v.p.minLength {
		return
	}

	// Early regex filtering - pre-compiled for efficiency
	if v.ignoreRegex != nil && v.ignoreRegex.MatchString(unquotedStr) {
		return
	}

	// Early number range filtering
	if v.p.numberMin != 0 || v.p.numberMax != 0 {
		if i, err := strconv.ParseInt(unquotedStr, 0, 0); err == nil {
			if (v.p.numberMin != 0 && i < int64(v.p.numberMin)) ||
				(v.p.numberMax != 0 && i > int64(v.p.numberMax)) {
				return
			}
		}
	}

	// Use interned string to reduce memory usage - identical strings share the same memory
	internedStr := InternString(unquotedStr)

	// Update the count first, this is faster than appending to slices
	count := v.p.IncrementStringCount(internedStr)

	// Only continue if we're still adding the position to the map
	// or if count has reached threshold
	if count == 1 || count == v.p.minOccurrences {
		// Lock to safely update the shared map
		v.p.stringMutex.Lock()
		defer v.p.stringMutex.Unlock()

		_, exists := v.p.strs[internedStr]
		if !exists {
			v.p.strs[internedStr] = make([]ExtendedPos, 0, v.p.minOccurrences) // Preallocate with expected size
		}

		// Create an optimized position record
		newPos := ExtendedPos{
			packageName: InternString(v.packageName), // Intern the package name to reduce memory
			Position:    v.fileSet.Position(pos),
		}

		v.p.strs[internedStr] = append(v.p.strs[internedStr], newPos)
	}
}

// addConst adds a const in the map along with its position in the tree.
func (v *treeVisitor) addConst(name string, val string, pos token.Pos) {
	// Early filtering using the same criteria as for strings
	var unquotedVal string
	if strings.HasPrefix(val, `"`) || strings.HasPrefix(val, "`") {
		var err error
		// Use string builder from pool to reduce allocations
		sb := GetStringBuilder()
		defer PutStringBuilder(sb)

		if unquotedVal, err = strconv.Unquote(val); err != nil {
			// If unquoting fails, manually strip quotes without allocations
			if len(val) >= 2 {
				sb.WriteString(val[1 : len(val)-1])
				unquotedVal = sb.String()
			} else {
				unquotedVal = val
			}
		}
	} else {
		unquotedVal = val
	}

	// Skip constants with values that would be filtered anyway
	if len(unquotedVal) < v.p.minLength {
		return
	}

	if v.ignoreRegex != nil && v.ignoreRegex.MatchString(unquotedVal) {
		return
	}

	// Use interned string to reduce memory usage
	internedVal := InternString(unquotedVal)
	internedName := InternString(name)
	internedPkg := InternString(v.packageName)

	// Lock to safely update the shared map
	v.p.constMutex.Lock()
	defer v.p.constMutex.Unlock()

	// track this const if this is a new const, or if we are searching for duplicate consts
	if _, ok := v.p.consts[internedVal]; !ok || v.p.findDuplicates {
		v.p.consts[internedVal] = append(v.p.consts[internedVal], ConstType{
			Name:        internedName,
			packageName: internedPkg,
			Position:    v.fileSet.Position(pos),
		})
	}
}

func (v *treeVisitor) isSupported(tk token.Token) bool {
	for _, s := range v.p.supportedTokens {
		if tk == s {
			return true
		}
	}
	return false
}

// evaluateConstExpr attempts to evaluate constant expressions.
// It handles cases like Prefix + "suffix" where both are constants.
// Returns the string value of the constant expression, or an empty string if not a string expression.
func (v *treeVisitor) evaluateConstExpr(expr ast.Expr) string {
	// Handle binary expressions like Prefix + "suffix"
	if binExpr, ok := expr.(*ast.BinaryExpr); ok && binExpr.Op == token.ADD {
		// We're only interested in string concatenation
		leftVal := v.resolveExprToString(binExpr.X)
		rightVal := v.resolveExprToString(binExpr.Y)
		
		// If both sides resolved to strings, combine them
		if leftVal != "" && rightVal != "" {
			return leftVal + rightVal
		}
	} else {
		// Handle single identifiers (could be constants)
		return v.resolveExprToString(expr)
	}
	
	return ""
}

// resolveExprToString tries to resolve an expression to its string value.
// Handles identifiers (looking up constants), string literals, and nested expressions.
func (v *treeVisitor) resolveExprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		// Direct string literal
		if e.Kind == token.STRING {
			val, err := strconv.Unquote(e.Value)
			if err == nil {
				return val
			}
			// Fall back to striping quotes manually if unquoting fails
			if len(e.Value) >= 2 {
				return e.Value[1 : len(e.Value)-1]
			}
		}
		
	case *ast.Ident:
		// Reference to a constant
		// Check if we've already seen this constant in the current package
		v.p.constMutex.RLock()
		defer v.p.constMutex.RUnlock()
		
		for val, constList := range v.p.consts {
			for _, c := range constList {
				// Match by name and package
				if c.Name == e.Name && c.packageName == v.packageName {
					return val
				}
			}
		}
		
	case *ast.BinaryExpr:
		// Recursively evaluate nested expressions
		if e.Op == token.ADD {
			left := v.resolveExprToString(e.X)
			right := v.resolveExprToString(e.Y)
			if left != "" && right != "" {
				return left + right
			}
		}
	
	case *ast.ParenExpr:
		// Handle parenthesized expressions
		return v.resolveExprToString(e.X)
	}
	
	return ""
}

// addConstWithValue adds a constant with an already evaluated string value.
// This is similar to addConst but skips the unquoting step since the value is already processed.
func (v *treeVisitor) addConstWithValue(name string, val string, pos token.Pos) {
	// Skip constants with values that would be filtered anyway
	if len(val) < v.p.minLength {
		return
	}

	if v.ignoreRegex != nil && v.ignoreRegex.MatchString(val) {
		return
	}

	// Use interned string to reduce memory usage
	internedVal := InternString(val)
	internedName := InternString(name)
	internedPkg := InternString(v.packageName)

	// Lock to safely update the shared map
	v.p.constMutex.Lock()
	defer v.p.constMutex.Unlock()

	// track this const if this is a new const, or if we are searching for duplicate consts
	if _, ok := v.p.consts[internedVal]; !ok || v.p.findDuplicates {
		v.p.consts[internedVal] = append(v.p.consts[internedVal], ConstType{
			Name:        internedName,
			packageName: internedPkg,
			Position:    v.fileSet.Position(pos),
		})
	}
}
