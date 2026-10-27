package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

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
	// cat=200 is the whole video section; sub-categories are narrowed below.
	u := a.base + "/q.php?cat=200&q=" + url.QueryEscape(query)
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
		ID       string `json:"id"`
		Name     string `json:"name"`
		InfoHash string `json:"info_hash"`
		Size     string `json:"size"`
		Seeders  string `json:"seeders"`
		Leechers string `json:"leechers"`
		Added    string `json:"added"`
		Category string `json:"category"`
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
		cat, _ := strconv.Atoi(r.Category)
		name := html.UnescapeString(r.Name)
		// Movies and TV only: no music videos, clips, "other", or NSFW.
		if !movieCats[cat] && !tvCats[cat] || isNSFW(name, cat) {
			continue
		}
		size, _ := strconv.ParseInt(r.Size, 10, 64)
		seeders, _ := strconv.Atoi(r.Seeders)
		leechers, _ := strconv.Atoi(r.Leechers)
		added, _ := strconv.ParseInt(r.Added, 10, 64)
		results = append(results, Result{
			ID:       r.ID,
			Name:     name,
			InfoHash: strings.ToLower(r.InfoHash),
			Size:     size,
			Seeders:  seeders,
			Leechers: leechers,
			Added:    time.Unix(added, 0),
			Category: cat,
			Meta:     classify(name, cat),
		})
	}
	return results, nil
}
