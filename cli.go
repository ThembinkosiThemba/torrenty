package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	maxShown       = 20
	reportInterval = time.Second
)

// runCLI is a plain line-based front end, used while the TUI is parked.
func runCLI(p Provider, d *Downloader) error {
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nsearch (q to quit)> ")
		if !in.Scan() {
			return in.Err()
		}
		q := strings.TrimSpace(in.Text())
		switch q {
		case "":
			continue
		case "q", "quit", "exit":
			return nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		fmt.Printf("searching %s…\n", p.Name())
		results, err := p.Search(ctx, q)
		cancel()
		if err != nil {
			fmt.Println("search failed:", err)
			continue
		}
		if len(results) == 0 {
			fmt.Printf("no results for %q\n", q)
			continue
		}
		printResults(results)

		fmt.Print("\n<n> download • t<n> save .torrent • m<n> open magnet • enter to search again\n> ")
		if !in.Scan() {
			return in.Err()
		}
		choice := strings.TrimSpace(in.Text())
		if choice == "" {
			continue
		}
		action := "d"
		if c := choice[0]; c == 't' || c == 'm' {
			action, choice = string(c), choice[1:]
		}
		n, err := strconv.Atoi(strings.TrimSpace(choice))
		if err != nil || n < 1 || n > min(len(results), maxShown) {
			fmt.Println("invalid choice")
			continue
		}
		r := results[n-1]

		switch action {
		case "m":
			if err := openTarget(r.Magnet()); err != nil {
				fmt.Println("could not open magnet:", err)
			} else {
				fmt.Println("opened magnet in default app")
			}
		default:
			if err := fetch(d, r, action == "t"); err != nil {
				fmt.Println("\nerror:", err)
			}
		}
	}
}

func printResults(results []Result) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "#\tName\tSize\tSeed\tLeech\tAdded\t")
	for i, r := range results[:min(len(results), maxShown)] {
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\t%d\t%s\t\n",
			i+1, truncate(r.Name, 60), humanBytes(r.Size), r.Seeders, r.Leechers, r.Added.Format("2006-01-02"))
	}
	w.Flush()
	if len(results) > maxShown {
		fmt.Printf("(showing %d of %d)\n", maxShown, len(results))
	}
}

// fetch downloads r's content, or only its .torrent file when saveOnly is set.
// Ctrl+C cancels the transfer and returns to the prompt.
func fetch(d *Downloader, r Result, saveOnly bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dl, err := d.Add(r)
	if err != nil {
		return err
	}
	defer d.Drop(dl)

	fmt.Println("fetching metadata (ctrl+c to cancel)…")
	err = d.WaitMetadata(ctx, dl, reportInterval, func(dl *Download) {
		fmt.Printf("\r  waiting for peers… %d connected   ", dl.Peers)
	})
	fmt.Println()
	if err != nil {
		return cancelled(err)
	}

	if saveOnly {
		path, err := d.SaveTorrentFile(dl)
		if err != nil {
			return err
		}
		fmt.Println("saved", path)
		return nil
	}

	fmt.Printf("downloading %s (%s) to %s\n", dl.Name, humanBytes(dl.Total), d.dir)
	err = d.Download(ctx, dl, reportInterval, func(dl *Download) {
		fmt.Printf("\r  %s %5.1f%%  %s / %s  %s/s  %d peers   ",
			bar(dl.Progress(), 25), dl.Progress()*100,
			humanBytes(dl.Done), humanBytes(dl.Total), humanBytes(int64(dl.Speed)), dl.Peers)
	})
	fmt.Println()
	if err != nil {
		return cancelled(err)
	}
	fmt.Println("done:", dl.Path)
	return nil
}

func cancelled(err error) error {
	if errors.Is(err, context.Canceled) {
		return errors.New("cancelled (partial files kept on disk)")
	}
	return err
}

func bar(frac float64, width int) string {
	n := int(frac * float64(width))
	return "[" + strings.Repeat("#", n) + strings.Repeat("-", width-n) + "]"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// openTarget opens a URL, magnet link or folder with the OS default handler.
func openTarget(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// rundll32 avoids cmd.exe mangling the '&' in magnet links.
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	case "darwin":
		cmd = exec.Command("open", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
