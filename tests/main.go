package main

import (
	"fmt"
	"strings"
)

const Foo = "bar"

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
