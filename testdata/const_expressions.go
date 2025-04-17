package testdata

const (
	Prefix = "example.com/"
	API    = Prefix + "api"
	Web    = Prefix + "web"
)

func testConstExpressions() {
	// These should match the constant expressions when using -eval-const-expr
	a := "example.com/api"
	b := "example.com/api"

	c := "example.com/web"
	d := "example.com/web"
}
