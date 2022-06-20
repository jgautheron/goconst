package main

import (
	"fmt"
	"strings"
)

const Foo = "bar"
const NumberConst = 123

var url string

func main() {
	if strings.HasPrefix(url, "http://") {
		url = strings.TrimPrefix(url, "http://")
	}
	url = strings.TrimPrefix(url, "/")
	fmt.Println(url)
}

func testCase() string {
	test := `test`
	if url == "test" {
		return test
	}
	switch url {
	case "moo":
		return ""
	}

	test2 := "moo" + fmt.Sprintf("%d", testInt())
	if test2 > "foo" {
		test2 += "foo"
	}
	return "foo" + test2
}

func testInt() int {
	test := 123
	if test == 123 {
		return 123
	}

	return 123
}
