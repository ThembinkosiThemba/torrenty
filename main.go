package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

func main() {
	dir := flag.String("dir", defaultDownloadDir(), "directory to save downloads and .torrent files in")
	flag.Parse()

	log.SetOutput(io.Discard)

	dl := newDownloader(*dir)
	defer dl.Close()

	if err := runCLI(newApibay(), dl); err != nil {
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
