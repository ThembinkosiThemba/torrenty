package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Result is a single search hit, independent of where it came from.
type Result struct {
	Name     string
	InfoHash string
	Size     int64
	Seeders  int
	Leechers int
	Added    time.Time
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

// Provider is a torrent search backend. Add new sources by implementing it.
type Provider interface {
	Name() string
	Search(ctx context.Context, query string) ([]Result, error)
}

// apibay talks to the public apibay.org JSON API.
type apibay struct {
	base   string
	client *http.Client
}

func newApibay() *apibay {
	return &apibay{
		base:   "https://apibay.org",
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (a *apibay) Name() string { return "apibay" }

func (a *apibay) Search(ctx context.Context, query string) ([]Result, error) {
	u := a.base + "/q.php?cat=0&q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "torrenty/0.1")

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: unexpected status %s", a.Name(), resp.Status)
	}

	// apibay encodes every number as a string.
	var raw []struct {
		Name     string `json:"name"`
		InfoHash string `json:"info_hash"`
		Size     string `json:"size"`
		Seeders  string `json:"seeders"`
		Leechers string `json:"leechers"`
		Added    string `json:"added"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("%s: decoding response: %w", a.Name(), err)
	}

	results := make([]Result, 0, len(raw))
	for _, r := range raw {
		// "No results" is signalled by a single all-zero hash entry.
		if strings.Trim(r.InfoHash, "0") == "" {
			continue
		}
		size, _ := strconv.ParseInt(r.Size, 10, 64)
		seeders, _ := strconv.Atoi(r.Seeders)
		leechers, _ := strconv.Atoi(r.Leechers)
		added, _ := strconv.ParseInt(r.Added, 10, 64)
		results = append(results, Result{
			Name:     r.Name,
			InfoHash: strings.ToLower(r.InfoHash),
			Size:     size,
			Seeders:  seeders,
			Leechers: leechers,
			Added:    time.Unix(added, 0),
		})
	}
	return results, nil
}

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
