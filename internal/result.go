// Package internal implements torrenty: search, classification, fetching
// .torrent files and the terminal UI.
package internal

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

// Provider is a torrent search backend. Add new sources by implementing it.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string) ([]Result, error)
}

// Result is a single search hit, independent of where it came from.
type Result struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	InfoHash string    `json:"info_hash"`
	Size     int64     `json:"size"`
	Seeders  int       `json:"seeders"`
	Leechers int       `json:"leechers"`
	Added    time.Time `json:"added"`
	Category int       `json:"category"`
	Meta     Meta      `json:"meta"`
}

// PageURL is the torrent's page on the site it came from.
func (r Result) PageURL() string {
	return "https://thepiratebay.org/description.php?id=" + r.ID
}

// Magnet builds a magnet URI for the result, including a few public trackers
// so peers can be found even when DHT is slow to bootstrap.
func (r Result) Magnet() string {
	v := url.Values{}
	v.Set("dn", r.Name)
	for _, tr := range trackers {
		v.Add("tr", tr)
	}
	return "magnet:?xt=urn:btih:" + r.InfoHash + "&" + v.Encode()
}

var trackers = []string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.stealth.si:80/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://open.demonii.com:1337/announce",
}

// humanBytes formats n as e.g. "1.4 GiB".
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
