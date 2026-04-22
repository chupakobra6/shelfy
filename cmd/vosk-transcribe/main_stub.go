//go:build !linux || !cgo

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "vosk-transcribe requires linux with cgo enabled")
	os.Exit(1)
}
