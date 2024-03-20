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
	return "foo"
}

func testInt() int {
	test := 123
	if test == 123 {
		return 123
	}

	return 123
}
