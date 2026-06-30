//go:build !linux

// typtel-tray is a Linux-only command (X11 + StatusNotifier). This stub keeps
// the package buildable on other platforms so `go build ./...` and
// `go test ./...` succeed there; it does nothing useful if run.
package main

import "fmt"

func main() {
	fmt.Println("typtel-tray is only supported on Linux (X11). Use typtel-menubar on macOS.")
}
