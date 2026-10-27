// Command torrenty is a small TUI for finding movies and series and saving
// their .torrent files.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"torrenty/internal"
)

func main() {
	dir := flag.String("dir", defaultDownloadDir(), "directory to save .torrent files in")
	flag.Parse()

	// Anything printed to the terminal would draw over the TUI.
	log.SetOutput(io.Discard)

	if err := internal.Run(*dir); err != nil {
		fmt.Fprintln(os.Stderr, "torrenty:", err)
		os.Exit(1)
	}
}

func defaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "torrenty-downloads"
	}
	return filepath.Join(home, "Downloads", "torrenty")
}
