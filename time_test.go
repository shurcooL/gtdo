package main

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
)

func ExampleMinuteRelTime() {
	then := time.Unix(0, 0)
	now0 := time.Unix(59, 999999999)
	now1 := now0.Add(1)

	fmt.Println(humanize.RelTime(then, now0, "ago", "from now"))
	fmt.Println(humanize.RelTime(then, now1, "ago", "from now"))
	fmt.Println()
	fmt.Println(minuteRelTime(then, now0))
	fmt.Println(minuteRelTime(then, now1))

	// Output:
	// 59 seconds ago
	// 1 minute ago
	//
	// less than a minute ago
	// 1 minute ago
}
