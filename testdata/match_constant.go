package testdata

// Constants in different forms
const SingleConst = "single constant"

const (
	// Grouped constants
	GroupedConst1 = "grouped constant"
	GroupedConst2 = "another grouped"

	// Constants with the same value
	DuplicateConst1 = "duplicate value"
	DuplicateConst2 = "duplicate value"

	// Constants with special characters
	SpecialConst = "special\nvalue\twith\rchars"
)

// Constants in different scopes
func scopedConstants() {
	const LocalConst = "local constant"
	str := "local constant" // Should match LocalConst

	if true {
		const BlockConst = "block constant"
		str := "block constant" // Should match BlockConst
	}
}

// Usage of constants from different contexts
func useConstants() {
	// Assignment context
	str1 := "single constant"             // Should match SingleConst
	str2 := "grouped constant"            // Should match GroupedConst1
	str3 := "duplicate value"             // Should match DuplicateConst1 (first defined)
	str4 := "special\nvalue\twith\rchars" // Should match SpecialConst

	// Binary expression context
	if str1 == "single constant" {
		println("matched")
	}

	// Case statement context
	switch str2 {
	case "grouped constant":
		println("matched")
	}

	// Function call context
	println("duplicate value")

	// Return statement context
	func() string {
		return "grouped constant"
	}()
}
