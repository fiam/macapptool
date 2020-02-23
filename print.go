package main

import (
	"fmt"
	"os"
)

func verbosePrintf(level int, format string, args ...interface{}) {
	if *verbose >= level {
		fmt.Printf(format, args...)
	}
}

func errPrintf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}
