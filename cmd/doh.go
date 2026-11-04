package main

import (
	"flag"
	doh "doh/internal"
)

func main() {
	// ponytail: parse flags in main(), not in an init(), so packages that
	// import startup remain testable (test flags would otherwise be parsed
	// before the testing package registers them).
	flag.Parse()
	doh.Run()
}
