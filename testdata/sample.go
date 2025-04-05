package testdata

import "fmt"

const (
	// This is already a constant
	ExistingConst = "already constant"

	// This is also a constant with the same value as duplicated strings below
	MatchingConst = "should be constant"
)

func stringDuplicates() {
	// These strings should be detected as duplicates
	str1 := "should be constant"
	str2 := "should be constant"

	// This exceeds the minimum occurrence threshold (assuming min=2)
	str3 := "another duplicate"
	str4 := "another duplicate"

	// These are too short to be detected (assuming min length=3)
	a := "ab"
	b := "ab"

	fmt.Println(str1, str2, str3, str4, a, b)
}

func numberDuplicates() {
	// These numbers should be detected as duplicates (when numbers flag is enabled)
	num1 := 12345
	num2 := 12345

	// These numbers are outside the min/max range (assuming min=100, max=1000)
	small := 50
	large := 2000

	fmt.Println(num1, num2, small, large)
}

func testContexts() {
	value := "test value"

	// Test various contexts for string detection

	// Assignment
	assigned := "test context"

	// Binary expression
	if value == "test context" {
		fmt.Println("Equal")
	}

	// Case clause
	switch value {
	case "test context":
		fmt.Println("Matched")
	}

	// Function call argument
	fmt.Println("test context")

	// Return statement
	func() string {
		return "test context"
	}()

	fmt.Println(assigned)
}

func testDuplicateConsts() {
	// This const should be detected as a duplicate const of MatchingConst
	const duplicate = "should be constant"
}
