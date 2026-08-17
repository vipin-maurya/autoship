//go:build ignore

// Command sleep blocks long enough for the runner's timeout test to cancel it.
package main

import "time"

func main() {
	time.Sleep(60 * time.Second)
}
