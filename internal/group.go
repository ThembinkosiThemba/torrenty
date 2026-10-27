package internal

import (
	"fmt"
	"sort"
)

type typeFilter int

const (
	moviesAndSeries typeFilter = iota
	moviesOnly
	seriesOnly
	typeFilters
)

func (t typeFilter) String() string {
	return [...]string{"movies + series", "movies", "series"}[t]
}

type sortMode int

const (
	bySmallest sortMode = iota
	bySeeders
	byNewest
	sortModes
)

func (s sortMode) String() string {
	return [...]string{"smallest first", "most seeders", "newest"}[s]
}

const minRes = 720

// Filters are the user-togglable result filters. The zero value is the default.
type Filters struct {
	Type        typeFilter
	AnyQuality  bool // include results below 720p
	IncludeDead bool // include results with no seeders
	Sort        sortMode
}

func (f Filters) keep(r Result) bool {
	series := r.Meta.Kind != KindMovie || tvCats[r.Category]
	switch {
	case f.Type == moviesOnly && series, f.Type == seriesOnly && !series:
		return false
	case !f.AnyQuality && r.Meta.Res < minRes:
		return false
	case !f.IncludeDead && r.Seeders == 0:
		return false
	}
	return true
}

func (f Filters) less(a, b Result) bool {
	switch f.Sort {
	case bySeeders:
		return a.Seeders > b.Seeders
	case byNewest:
		return a.Added.After(b.Added)
	}
	return a.Size < b.Size
}

// row is one line of the results list: either a heading or a result.
type row struct {
	heading string
	depth   int
	r       *Result
}

func (r row) selectable() bool { return r.r != nil }

// buildRows filters results and arranges them into a grouped list:
//
//	Breaking Bad
//	  Complete / multi-season packs
//	  Season 1
//	    Season pack
//	    Episodes
//	Movies
//
// Season headings are only shown when a show has more than one season.
func buildRows(results []Result, f Filters) (rows []row, kept int) {
	var movies, other []*Result
	shows := map[string]*showGroup{}
	var showOrder []string

	for i := range results {
		r := &results[i]
		if !f.keep(*r) {
			continue
		}
		kept++
		switch {
		case r.Meta.Kind != KindMovie:
			g := shows[r.Meta.ShowKey]
			if g == nil {
				g = &showGroup{title: r.Meta.Show, seasons: map[int]*seasonGroup{}}
				shows[r.Meta.ShowKey] = g
				showOrder = append(showOrder, r.Meta.ShowKey)
			}
			g.add(r)
		case tvCats[r.Category]:
			other = append(other, r) // TV category but no season info in the name
		default:
			movies = append(movies, r)
		}
	}

	// Shows with the most results first; that's usually what was searched for.
	sort.SliceStable(showOrder, func(i, j int) bool {
		return shows[showOrder[i]].count > shows[showOrder[j]].count
	})
	for _, key := range showOrder {
		rows = append(rows, shows[key].rows(f)...)
	}

	section := func(title string, rs []*Result) {
		if len(rs) == 0 {
			return
		}
		sortResults(rs, f, false)
		rows = append(rows, row{heading: fmt.Sprintf("%s (%d)", title, len(rs))})
		for _, r := range rs {
			rows = append(rows, row{r: r, depth: 1})
		}
	}
	section("Movies", movies)
	section("Other TV", other)
	return rows, kept
}

type showGroup struct {
	title   string
	count   int
	multi   []*Result
	seasons map[int]*seasonGroup
}

type seasonGroup struct {
	packs, episodes []*Result
}

func (g *showGroup) add(r *Result) {
	g.count++
	if r.Meta.Kind == KindMultiSeason {
		g.multi = append(g.multi, r)
		return
	}
	s := g.seasons[r.Meta.Season]
	if s == nil {
		s = &seasonGroup{}
		g.seasons[r.Meta.Season] = s
	}
	if r.Meta.Kind == KindSeason {
		s.packs = append(s.packs, r)
	} else {
		s.episodes = append(s.episodes, r)
	}
}

func (g *showGroup) rows(f Filters) []row {
	rows := []row{{heading: fmt.Sprintf("%s (%d)", g.title, g.count)}}
	sub := func(title string, depth int, rs []*Result, byEpisode bool) {
		if len(rs) == 0 {
			return
		}
		sortResults(rs, f, byEpisode)
		rows = append(rows, row{heading: fmt.Sprintf("%s (%d)", title, len(rs)), depth: depth})
		for _, r := range rs {
			rows = append(rows, row{r: r, depth: depth + 1})
		}
	}

	sub("Complete / multi-season packs", 1, g.multi, false)

	nums := make([]int, 0, len(g.seasons))
	for n := range g.seasons {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	multiSeason := len(nums) > 1
	for _, n := range nums {
		s := g.seasons[n]
		depth := 1
		if multiSeason {
			rows = append(rows, row{heading: fmt.Sprintf("Season %d", n), depth: 1})
			depth = 2
		}
		sub("Season pack", depth, s.packs, false)
		sub("Episodes", depth, s.episodes, true)
	}
	return rows
}

// sortResults orders rs by the chosen sort. Episodes are kept in episode
// order, with the chosen sort applied among releases of the same episode.
func sortResults(rs []*Result, f Filters, byEpisode bool) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if byEpisode && a.Meta.Episode != b.Meta.Episode {
			return a.Meta.Episode < b.Meta.Episode
		}
		return f.less(*a, *b)
	})
}
