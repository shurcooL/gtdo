package datad

import (
	"flag"
	"log"
)

var debug = flag.Bool("test.debug", false, "print datad debug log messages")

func init() {
	if *debug {
		log.SetFlags(log.Lshortfile)
	}
}
