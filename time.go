package main

import (
	"time"

	"github.com/dustin/go-humanize"
)

// minuteTime is like humanize.Time, but it returns "less than a minute ago" for durations under a minute.
func minuteTime(then time.Time) string {
	return minuteRelTime(then, time.Now())
}

func minuteRelTime(then, now time.Time) string {
	if now.Sub(then) < time.Minute {
		return "less than a minute ago"
	}
	return humanize.RelTime(then, now, "ago", "from now")
}
