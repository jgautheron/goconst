#!/bin/bash
set -e

# This script tests compatibility of goconst with golangci-lint
# using the command line interface rather than importing the package

echo "Testing compatibility with golangci-lint..."

# Get current directory
REPO_ROOT="$(pwd)"
echo "Using repository at: $REPO_ROOT"

# Set up test directory
TEST_DIR=$(mktemp -d)
echo "Created test directory: $TEST_DIR"

# Clean up on exit
cleanup() {
  echo "Cleaning up test directory..."
  rm -rf "$TEST_DIR"
}
trap cleanup EXIT

# Build goconst CLI
echo "Building goconst CLI..."
GOCONST_BIN="$TEST_DIR/goconst"
go build -o "$GOCONST_BIN" ./cmd/goconst

# Create test files
echo "Creating test files..."
mkdir -p "$TEST_DIR/testpkg"

# Create a file with constants and strings
cat > "$TEST_DIR/testpkg/main.go" << 'EOF'
package testpkg

const ExistingConst = "test-const"

func example() {
    // This should be detected as it matches ExistingConst
    str1 := "test-const"
    str2 := "test-const"

    // This should be detected as a duplicate without matching constant
    dup1 := "duplicate"
    dup2 := "duplicate"

    // This should be ignored due to ignore-strings
    skip := "test-ignore"
    skip2 := "test-ignore"
    
    // These should be ignored with the multiple pattern test
    foo1 := "foo-prefix"
    foo2 := "foo-prefix"
    
    bar1 := "bar-prefix"
    bar2 := "bar-prefix"
}
EOF

# Test 1: Basic functionality
echo "Test 1: Basic functionality (without match-with-constants)..."
"$GOCONST_BIN" -ignore-strings "test-ignore" -match-constant=false "$TEST_DIR/testpkg" > "$TEST_DIR/output1.txt"
if ! grep -q "duplicate" "$TEST_DIR/output1.txt"; then
    echo "Failed: Should detect 'duplicate' string"
    cat "$TEST_DIR/output1.txt"
    exit 1
fi
if ! grep -q "test-const" "$TEST_DIR/output1.txt"; then
    echo "Failed: Should detect 'test-const' string"
    cat "$TEST_DIR/output1.txt"
    exit 1
fi
if grep -q "test-ignore" "$TEST_DIR/output1.txt"; then
    echo "Failed: Should NOT detect 'test-ignore' string"
    cat "$TEST_DIR/output1.txt"
    exit 1
fi

# Test 2: Match with constants
echo "Test 2: Testing match-with-constants functionality..."
"$GOCONST_BIN" -ignore-strings "test-ignore" -match-constant "$TEST_DIR/testpkg" > "$TEST_DIR/output2.txt"
if ! grep -q "matching constant.*ExistingConst" "$TEST_DIR/output2.txt"; then
    echo "Failed: Should match 'test-const' with 'ExistingConst'"
    cat "$TEST_DIR/output2.txt"
    exit 1
fi
if ! grep -q "duplicate" "$TEST_DIR/output2.txt"; then
    echo "Failed: Should detect 'duplicate' string"
    cat "$TEST_DIR/output2.txt"
    exit 1
fi
if grep -q "test-ignore" "$TEST_DIR/output2.txt"; then
    echo "Failed: Should NOT detect 'test-ignore' string"
    cat "$TEST_DIR/output2.txt"
    exit 1
fi

# Test 3: Create another test file with multiple constants
cat > "$TEST_DIR/testpkg/multi_const.go" << 'EOF'
package testpkg

const (
    FirstConst = "duplicate-value"
    SecondConst = "duplicate-value"
)

func multipleConstants() {
    x := "duplicate-value"
    y := "duplicate-value"
}
EOF

echo "Test 3: Testing multiple constants with same value..."
"$GOCONST_BIN" -match-constant "$TEST_DIR/testpkg" > "$TEST_DIR/output3.txt"
if ! grep -q "matching constant.*FirstConst" "$TEST_DIR/output3.txt"; then
    echo "Failed: Should match 'duplicate-value' with 'FirstConst'"
    cat "$TEST_DIR/output3.txt"
    exit 1
fi

# Test 4: Test with JSON output (golangci-lint compatibility)
echo "Test 4: Testing JSON output format..."
"$GOCONST_BIN" -ignore-strings "test-ignore" -match-constant -output json "$TEST_DIR/testpkg" > "$TEST_DIR/output4.json"
# Check that the JSON has the correct structure: strings + constants sections
if ! grep -q '"constants".*"test-const"' "$TEST_DIR/output4.json"; then
    echo "Failed: JSON output should include constants with test-const"
    cat "$TEST_DIR/output4.json"
    exit 1
fi

if ! grep -q '"Name":"ExistingConst"' "$TEST_DIR/output4.json"; then
    echo "Failed: JSON output should include ExistingConst"
    cat "$TEST_DIR/output4.json"
    exit 1
fi

# Test 5: Test with multiple ignore patterns (comma-separated)
echo "Test 5: Testing multiple ignore patterns..."
"$GOCONST_BIN" -ignore-strings "test-ignore,foo-prefix,bar-prefix" "$TEST_DIR/testpkg" > "$TEST_DIR/output5.txt"
if grep -q "test-ignore" "$TEST_DIR/output5.txt"; then
    echo "Failed: Should NOT detect 'test-ignore' string"
    cat "$TEST_DIR/output5.txt"
    exit 1
fi
if grep -q "foo-prefix" "$TEST_DIR/output5.txt"; then
    echo "Failed: Should NOT detect 'foo-prefix' string"
    cat "$TEST_DIR/output5.txt"
    exit 1
fi
if grep -q "bar-prefix" "$TEST_DIR/output5.txt"; then
    echo "Failed: Should NOT detect 'bar-prefix' string"
    cat "$TEST_DIR/output5.txt"
    exit 1
fi
if ! grep -q "duplicate" "$TEST_DIR/output5.txt"; then
    echo "Failed: Should detect 'duplicate' string"
    cat "$TEST_DIR/output5.txt"
    exit 1
fi

# Create a file with constant expressions for testing
cat > "$TEST_DIR/testpkg/const_expr.go" << 'EOF'
package testpkg

const (
    Prefix = "domain.com/"
    API = Prefix + "api"
    Web = Prefix + "web"
)

func constExpressions() {
    // These should be detected with matching constants when using -eval-const-expr
    path1 := "domain.com/api"
    path2 := "domain.com/api"

    web1 := "domain.com/web"
    web2 := "domain.com/web"

    // This is just the prefix, not a compound expression result
    prefix := "domain.com/"
}
EOF

# Test 6: Test constant expression evaluation
echo "Test 6: Testing constant expression evaluation..."
"$GOCONST_BIN" -match-constant -eval-const-expr "$TEST_DIR/testpkg" > "$TEST_DIR/output6.txt"
if ! grep -q "matching constant.*API" "$TEST_DIR/output6.txt"; then
    echo "Failed: Should match 'domain.com/api' with 'API' constant"
    cat "$TEST_DIR/output6.txt"
    exit 1
fi
if ! grep -q "matching constant.*Web" "$TEST_DIR/output6.txt"; then
    echo "Failed: Should match 'domain.com/web' with 'Web' constant"
    cat "$TEST_DIR/output6.txt"
    exit 1
fi

echo "All compatibility tests PASSED!" 