package internal

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kind is what a torrent contains, worked out from its name and category.
type Kind int

const (
	KindMovie       Kind = iota
	KindEpisode          // one episode (or a short run, S01E01-E03)
	KindSeason           // a single full season
	KindMultiSeason      // several seasons, or "complete series"
)

// Meta is what we could parse out of a release name.
type Meta struct {
	Kind       Kind
	Show       string // human title, e.g. "Breaking Bad"
	ShowKey    string // normalised title used for grouping
	Season     int    // first season (0 if unknown)
	SeasonEnd  int    // last season for multi-season packs (0 if open-ended)
	Episode    int
	EpisodeEnd int
	Res        int    // vertical resolution; 0 means SD/unknown
	ResLabel   string // "1080p", "HD", "SD", …
}

// apibay video sub-categories.
var (
	movieCats = map[int]bool{201: true, 202: true, 207: true, 209: true, 211: true}
	tvCats    = map[int]bool{205: true, 208: true, 212: true}
)

var (
	reEpisode     = regexp.MustCompile(`(?i)\bS(\d{1,2}) ?E(\d{1,3})(?:-?E(\d{1,3}))?`)
	reXEpisode    = regexp.MustCompile(`(?i)\b(\d{1,2})x(\d{2,3})\b`)
	reSeasonRange = regexp.MustCompile(`(?i)\bS(\d{1,2}) ?(?:-|to) ?S?(\d{1,2})\b`)
	reSeasonToken = regexp.MustCompile(`(?i)\bS(\d{1,2})\b`)
	reSeasonWord  = regexp.MustCompile(`(?i)\bseasons? ?(\d{1,2})(?: ?(?:-|to|&|and) ?(\d{1,2}))?\b`)
	reComplete    = regexp.MustCompile(`(?i)\bcomplete\b`)
	reRes         = regexp.MustCompile(`(?i)\b(2160|1440|1080|720|576|480|360)[pi]\b`)
	reUHD         = regexp.MustCompile(`(?i)\b(4k|uhd)\b`)
	reYear        = regexp.MustCompile(`\(?\b(19|20)\d{2}\b\)?`)
	reNonWord     = regexp.MustCompile(`[^\pL\pN]+`)
	reNSFW        = regexp.MustCompile(`(?i)\b(xxx|porn|onlyfans|brazzers|hentai)\b`)
)

func classify(name string, category int) Meta {
	// Separators vary between releases; normalise so \b works everywhere.
	clean := strings.NewReplacer(".", " ", "_", " ", "[", " ", "]", " ").Replace(name)

	m := Meta{}
	m.Res, m.ResLabel = resolution(clean, category)

	cut := -1 // where the show title ends
	mark := func(loc []int) {
		if loc != nil && (cut < 0 || loc[0] < cut) {
			cut = loc[0]
		}
	}

	if sm := reEpisode.FindStringSubmatchIndex(clean); sm != nil {
		m.Kind = KindEpisode
		m.Season = atoi(clean, sm, 1)
		m.Episode = atoi(clean, sm, 2)
		m.EpisodeEnd = atoi(clean, sm, 3)
		mark(sm)
	} else if sm := reXEpisode.FindStringSubmatchIndex(clean); sm != nil && (tvCats[category] || category == 0) {
		m.Kind = KindEpisode
		m.Season = atoi(clean, sm, 1)
		m.Episode = atoi(clean, sm, 2)
		mark(sm)
	} else {
		// Collect every season number mentioned, then decide pack vs multi.
		lo, hi := 0, 0
		add := func(n int) {
			if n <= 0 {
				return
			}
			if lo == 0 || n < lo {
				lo = n
			}
			if n > hi {
				hi = n
			}
		}
		for _, sm := range reSeasonRange.FindAllStringSubmatchIndex(clean, -1) {
			add(atoi(clean, sm, 1))
			add(atoi(clean, sm, 2))
			mark(sm)
		}
		for _, sm := range reSeasonToken.FindAllStringSubmatchIndex(clean, -1) {
			add(atoi(clean, sm, 1))
			mark(sm)
		}
		for _, sm := range reSeasonWord.FindAllStringSubmatchIndex(clean, -1) {
			add(atoi(clean, sm, 1))
			add(atoi(clean, sm, 2))
			mark(sm)
		}
		complete := reComplete.FindStringIndex(clean)

		switch {
		case lo > 0 && hi > lo:
			m.Kind, m.Season, m.SeasonEnd = KindMultiSeason, lo, hi
		case lo > 0:
			m.Kind, m.Season = KindSeason, lo
		case complete != nil && tvCats[category]:
			// "Complete Series" with no season numbers at all.
			m.Kind = KindMultiSeason
			mark(complete)
		}
	}

	if m.Kind != KindMovie {
		title := clean
		if cut >= 0 {
			title = clean[:cut]
		}
		m.Show, m.ShowKey = showTitle(title)
	}
	return m
}

func resolution(name string, category int) (int, string) {
	if sm := reRes.FindStringSubmatch(name); sm != nil {
		n, _ := strconv.Atoi(sm[1])
		return n, sm[1] + "p"
	}
	if reUHD.MatchString(name) {
		return 2160, "2160p"
	}
	// No tag in the name: trust the site's HD/UHD categories.
	switch category {
	case 211, 212:
		return 2160, "UHD"
	case 207, 208:
		return 720, "HD"
	}
	return 0, "SD"
}

func showTitle(s string) (display, key string) {
	s = reYear.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(reNonWord.ReplaceAllString(s, " ")), " ")
	if s == "" {
		return "Unknown show", ""
	}
	return s, strings.ToLower(s)
}

func atoi(s string, sm []int, group int) int {
	if 2*group+1 >= len(sm) || sm[2*group] < 0 {
		return 0
	}
	n, _ := strconv.Atoi(s[sm[2*group]:sm[2*group+1]])
	return n
}

// isNSFW is a belt-and-braces check on top of only querying video categories.
func isNSFW(name string, category int) bool {
	return (category >= 500 && category < 600) || reNSFW.MatchString(name)
}

// tag is a short label for the result, e.g. "S01E04", "S02" or "S01-S05".
func (m Meta) tag() string {
	switch m.Kind {
	case KindEpisode:
		if m.EpisodeEnd > m.Episode {
			return fmt.Sprintf("S%02dE%02d-%02d", m.Season, m.Episode, m.EpisodeEnd)
		}
		return fmt.Sprintf("S%02dE%02d", m.Season, m.Episode)
	case KindSeason:
		return fmt.Sprintf("S%02d", m.Season)
	case KindMultiSeason:
		if m.Season > 0 {
			return fmt.Sprintf("S%02d-S%02d", m.Season, m.SeasonEnd)
		}
		return "complete"
	}
	return ""
}

// kindName is a human name for m.Kind.
func (m Meta) kindName() string {
	return [...]string{"Movie", "Episode", "Season pack", "Multi-season pack"}[m.Kind]
}
