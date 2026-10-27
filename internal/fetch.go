package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// stallTimeout makes aria2 give up when no peer has sent metadata for this
// many seconds, so a dead torrent doesn't hang forever.
const stallTimeout = "120"

// Fetcher saves .torrent files by asking aria2c to fetch a magnet's metadata.
type Fetcher struct {
	dir string
	bin string // path to aria2c, empty if not found
}

func newFetcher(dir string) *Fetcher {
	return &Fetcher{dir: dir, bin: findAria2()}
}

func (f *Fetcher) Available() bool { return f.bin != "" }

// findAria2 looks on PATH first, then in winget's package folder, since a
// fresh winget install isn't on PATH until the shell is restarted.
func findAria2() string {
	if bin, err := exec.LookPath("aria2c"); err == nil {
		return bin
	}
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(local, "Microsoft", "WinGet", "Packages", "aria2.aria2_*", "*", "aria2c.exe"))
	if len(matches) == 0 {
		return ""
	}
	return matches[len(matches)-1] // Glob sorts, so this is the newest version
}

// SaveTorrent fetches r's metadata from peers and writes it as a .torrent
// file in the download directory. Cancelling ctx kills aria2c.
func (f *Fetcher) SaveTorrent(ctx context.Context, r Result) (string, error) {
	if !f.Available() {
		return "", errors.New("aria2c not found — install it with: winget install aria2.aria2")
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		return "", err
	}

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, f.bin,
		"--dir="+f.dir,
		"--bt-metadata-only=true",
		"--bt-save-metadata=true",
		"--bt-stop-timeout="+stallTimeout,
		"--allow-overwrite=true",
		"--auto-file-renaming=false",
		"--summary-interval=0",
		"--console-log-level=error",
		"--download-result=hide",
		r.Magnet(),
	)
	// aria2c must never touch the terminal while the TUI owns it.
	cmd.Stdout, cmd.Stderr = &out, &out

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", errors.New("cancelled")
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if exit.ExitCode() == 2 {
				return "", errors.New("timed out: no peers sent metadata")
			}
			if msg := lastLine(out.String()); msg != "" {
				return "", fmt.Errorf("aria2c: %s", msg)
			}
			return "", fmt.Errorf("aria2c exited with code %d", exit.ExitCode())
		}
		return "", err
	}

	// aria2 names the file after the info hash; give it a readable name.
	src := filepath.Join(f.dir, r.InfoHash+".torrent")
	dst := f.targetPath(r)
	if err := os.Rename(src, dst); err != nil {
		return src, nil
	}
	return dst, nil
}

// targetPath is where SaveTorrent writes r's .torrent file.
func (f *Fetcher) targetPath(r Result) string {
	return filepath.Join(f.dir, sanitizeFilename(r.Name)+".torrent")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func sanitizeFilename(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, s)
	s = strings.TrimRight(s, ". ")
	if s == "" {
		s = "torrent"
	}
	return s
}
