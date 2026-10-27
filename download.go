package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
)

const metadataTimeout = 3 * time.Minute

// Download tracks one torrent added to the client.
type Download struct {
	Name     string
	InfoHash string
	Path     string // where the content or .torrent file ended up

	Done, Total int64
	Speed       float64 // bytes/sec
	Peers       int

	t *torrent.Torrent
}

func (d *Download) Progress() float64 {
	if d.Total == 0 {
		return 0
	}
	return float64(d.Done) / float64(d.Total)
}

// Downloader owns the torrent client, created lazily so that just searching
// never opens listening ports.
type Downloader struct {
	dir    string
	client *torrent.Client
}

func newDownloader(dir string) *Downloader {
	return &Downloader{dir: dir}
}

func (d *Downloader) ensureClient() error {
	if d.client != nil {
		return nil
	}
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return err
	}
	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = d.dir
	// Keep the library quiet; its logs would interleave with our output.
	cfg.Slogger = slog.New(slog.DiscardHandler)
	c, err := torrent.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("starting torrent client: %w", err)
	}
	d.client = c
	return nil
}

// Add registers r with the client. Nothing is downloaded until
// WaitMetadata / Download are called.
func (d *Downloader) Add(r Result) (*Download, error) {
	if err := d.ensureClient(); err != nil {
		return nil, err
	}
	t, err := d.client.AddMagnet(r.Magnet())
	if err != nil {
		return nil, err
	}
	return &Download{Name: r.Name, InfoHash: r.InfoHash, Total: r.Size, t: t}, nil
}

// WaitMetadata blocks until the torrent's info dictionary arrives from peers,
// calling progress every interval so callers can show the peer count.
func (d *Downloader) WaitMetadata(ctx context.Context, dl *Download, interval time.Duration, progress func(*Download)) error {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-dl.t.GotInfo():
			dl.Name = dl.t.Name()
			dl.Total = dl.t.Length()
			return nil
		case <-tick.C:
			dl.Peers = dl.t.Stats().ActivePeers
			progress(dl)
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return errors.New("timed out finding peers")
			}
			return ctx.Err()
		}
	}
}

// SaveTorrentFile writes the .torrent file for dl. Metadata must be present.
func (d *Downloader) SaveTorrentFile(dl *Download) (string, error) {
	path := filepath.Join(d.dir, sanitizeFilename(dl.Name)+".torrent")
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	mi := dl.t.Metainfo()
	if err := mi.Write(f); err != nil {
		f.Close()
		return "", err
	}
	dl.Path = path
	return path, f.Close()
}

// Download fetches all content for dl, calling progress every interval.
// Metadata must be present. Returns when complete or ctx is cancelled.
func (d *Downloader) Download(ctx context.Context, dl *Download, interval time.Duration, progress func(*Download)) error {
	dl.Path = filepath.Join(d.dir, dl.Name)
	dl.t.DownloadAll()
	tick := time.NewTicker(interval)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			done := dl.t.BytesCompleted()
			dl.Speed = float64(done-dl.Done) / interval.Seconds()
			dl.Done = done
			dl.Peers = dl.t.Stats().ActivePeers
			progress(dl)
			if dl.Total > 0 && done >= dl.Total {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Drop stops a torrent. Files already on disk are kept.
func (d *Downloader) Drop(dl *Download) {
	if dl.t != nil {
		dl.t.Drop()
		dl.t = nil
	}
}

func (d *Downloader) Close() {
	if d.client != nil {
		d.client.Close()
	}
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
