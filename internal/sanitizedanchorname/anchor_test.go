package sanitizedanchorname_test

import (
	"fmt"

	"github.com/shurcooL/gtdo/internal/sanitizedanchorname"
)

func ExampleCreate() {
	anchorName := sanitizedanchorname.Create("This is a header")

	fmt.Println(anchorName)

	// Output:
	// this-is-a-header
}

func ExampleCreate_two() {
	fmt.Println(sanitizedanchorname.Create("This is a header"))
	fmt.Println(sanitizedanchorname.Create("This is also          a header"))
	fmt.Println(sanitizedanchorname.Create("main.go"))
	fmt.Println(sanitizedanchorname.Create("Article 123"))
	fmt.Println(sanitizedanchorname.Create("<- Let's try this, shall we?"))
	fmt.Printf("%q\n", sanitizedanchorname.Create("        "))
	fmt.Println(sanitizedanchorname.Create("Hello, 世界"))

	// Output:
	// this-is-a-header
	// this-is-also-a-header
	// main.go
	// article-123
	// let-s-try-this-shall-we
	// ""
	// hello-世界
}
