package internal

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	accent = lipgloss.Color("#7D56F4")
	subtle = lipgloss.Color("241")

	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(accent).Padding(0, 1)
	tabStyle     = lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)
	activeTab    = lipgloss.NewStyle().Foreground(accent).Bold(true).Underline(true).Padding(0, 1)
	helpStyle    = lipgloss.NewStyle().Foreground(subtle)
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	okStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dimStyle     = lipgloss.NewStyle().Foreground(subtle)
	nameStyle    = lipgloss.NewStyle().Bold(true)
	selectedBar  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	selectedRow  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA")).Background(accent)
	keyStyle     = lipgloss.NewStyle().Foreground(accent).Bold(true)
	headingStyle = []lipgloss.Style{
		lipgloss.NewStyle().Foreground(accent).Bold(true),
		lipgloss.NewStyle().Bold(true),
		lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Italic(true),
	}
	resStyle = map[int]lipgloss.Style{
		2160: lipgloss.NewStyle().Foreground(lipgloss.Color("213")),
		1080: lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
	}
)

// Fixed layout of the search view, in lines.
const (
	listTop     = 6 // header, gap, input, filter bar, gap, column header
	footerLines = 4 // gap, details, status, help
	colsWidth   = 43
)

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	var lines []string
	switch m.view {
	case focusView:
		lines = m.focusLines()
	case savedView:
		lines = append([]string{m.header(), ""}, m.savedLines()...)
	default:
		lines = m.searchLines()
	}
	footer := m.footer()
	for len(lines) < m.height-len(footer) {
		lines = append(lines, "")
	}
	return strings.Join(append(lines[:min(len(lines), max(m.height-len(footer), 0))], footer...), "\n")
}

func (m model) header() string {
	searchTab, savedTab := tabStyle, tabStyle
	if m.view == searchView {
		searchTab = activeTab
	} else {
		savedTab = activeTab
	}
	savedLabel := "Saved"
	if n := len(m.jobs); n > 0 {
		savedLabel = fmt.Sprintf("Saved (%d)", n)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		titleStyle.Render("torrenty"), " ",
		searchTab.Render("Search"),
		savedTab.Render(savedLabel),
	)
}

func (m model) searchLines() []string {
	lines := []string{m.header(), "", m.input.View(), m.filterBar(), ""}
	if len(m.rows) == 0 {
		return lines
	}
	lines = append(lines, dimStyle.Render(fit("", m.width-colsWidth-1)+" "+columns("Res", "Size", "Seed", "Leech", "Added")))
	end := min(m.offset+m.listHeight(), len(m.rows))
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.renderRow(i))
	}
	return lines
}

func (m model) filterBar() string {
	quality := "≥720p"
	if m.filters.AnyQuality {
		quality = "any quality"
	}
	dead := "seeded only"
	if m.filters.IncludeDead {
		dead = "incl. dead"
	}
	item := func(key, label string) string {
		return keyStyle.Render(key) + " " + label
	}
	return fit("  "+strings.Join([]string{
		item("c", m.filters.Type.String()),
		item("a", quality),
		item("d", dead),
		item("s", m.filters.Sort.String()),
	}, dimStyle.Render("  ·  ")), m.width)
}

func (m model) renderRow(i int) string {
	rw := m.rows[i]
	indent := strings.Repeat("  ", rw.depth)
	if !rw.selectable() {
		style := headingStyle[min(rw.depth, len(headingStyle)-1)]
		return style.Render(fit("  "+indent+rw.heading, m.width))
	}

	r := rw.r
	nameW := m.width - colsWidth - 1
	label := r.Name
	if tag := r.Meta.tag(); tag != "" {
		label = fmt.Sprintf("%-9s %s", tag, label) // widest tag: S01E01-03
	}
	added := r.Added.Format("2006-01-02")
	cols := columns(r.Meta.ResLabel, humanBytes(r.Size), fmt.Sprint(r.Seeders), fmt.Sprint(r.Leechers), added)

	if i == m.cursor {
		return selectedRow.Render(fit("▌ "+indent+label, nameW) + " " + cols)
	}
	res := fmt.Sprintf("%6s", r.Meta.ResLabel)
	if st, ok := resStyle[r.Meta.Res]; ok {
		res = st.Render(res)
	}
	return fit("  "+indent+label, nameW) + " " + res + dimStyle.Render(cols[6:])
}

// columns lays out the fixed-width right-hand columns (colsWidth wide).
func columns(res, size, seed, leech, added string) string {
	return fmt.Sprintf("%6s %10s %7s %6s %10s", res, size, seed, leech, added)
}

func (m model) savedLines() []string {
	if len(m.jobs) == 0 {
		return []string{dimStyle.Render("  nothing yet — pick a result and press enter to save its .torrent file")}
	}
	var lines []string
	for i, j := range m.jobs {
		cursor := "  "
		if i == m.jobCursor {
			cursor = selectedBar.Render("▌ ")
		}
		lines = append(lines, cursor+nameStyle.Render(fit(j.r.Name, m.width-4)), "  "+m.jobLine(j), "")
	}
	return lines
}

func (m model) jobLine(j *job) string {
	switch j.state {
	case jobRunning:
		secs := int(time.Since(j.started).Seconds())
		return dimStyle.Render(fmt.Sprintf("%s fetching metadata from peers… %ds", m.spinner.View(), secs))
	case jobDone:
		when := ""
		if !j.savedAt.IsZero() {
			when = j.savedAt.Format("2 Jan 15:04") + " · "
		}
		return okStyle.Render("✓ saved ") + dimStyle.Render(when+j.path)
	default:
		return errStyle.Render("✗ " + j.err.Error())
	}
}

// focusLines shows a single torrent and nothing else.
func (m model) focusLines() []string {
	j := m.focused
	r := j.r
	w := max(m.width-4, 20)

	lines := []string{titleStyle.Render("torrenty") + dimStyle.Render("  ›  save .torrent"), ""}
	for _, l := range strings.Split(lipgloss.NewStyle().Width(w).Render(r.Name), "\n") {
		lines = append(lines, "  "+nameStyle.Render(l))
	}
	lines = append(lines, "")

	field := func(k, v string) {
		lines = append(lines, "  "+dimStyle.Render(fmt.Sprintf("%-9s", k))+v)
	}
	field("Type", r.Meta.kindName()+" "+dimStyle.Render(r.Meta.tag()))
	if r.Meta.Kind != KindMovie {
		field("Show", r.Meta.Show)
	}
	field("Quality", r.Meta.ResLabel)
	field("Size", humanBytes(r.Size))
	field("Peers", fmt.Sprintf("%d seeders · %d leechers", r.Seeders, r.Leechers))
	field("Added", r.Added.Format("2 Jan 2006"))
	field("Hash", dimStyle.Render(r.InfoHash))
	if j.confirm {
		return append(lines, "",
			"  "+warnStyle.Render("⚠ you already have this .torrent"),
			"  "+dimStyle.Render(j.path),
			"",
			"  "+keyStyle.Render("r")+" replace with a fresh copy   "+keyStyle.Render("esc")+" keep existing")
	}
	lines = append(lines, "", "  "+m.jobLine(j))
	if j.state == jobDone && j.took > 0 {
		lines = append(lines, "  "+dimStyle.Render(fmt.Sprintf("took %s", j.took.Round(time.Second))))
	}
	return lines
}

// details describes the highlighted result, shown above the status line.
func (m model) details() string {
	r := m.selected()
	if m.view != searchView || r == nil {
		return ""
	}
	d := r.Meta.kindName()
	if r.Meta.Kind != KindMovie {
		d += " · " + r.Meta.Show
	}
	return dimStyle.Render(fit(fmt.Sprintf("  %s · %s · %s", d, r.InfoHash, r.PageURL()), m.width))
}

func (m model) footer() []string {
	status := m.status
	if m.busy() {
		status = m.spinner.View() + " " + status
	}
	statusStyle := dimStyle
	if m.statusErr {
		statusStyle = errStyle
	}

	var help string
	switch {
	case m.view == focusView && m.focused.confirm:
		help = "r replace • enter open existing • f open folder • esc/q cancel"
	case m.view == focusView:
		switch m.focused.state {
		case jobDone:
			help = "enter open .torrent • f open folder • m magnet • o page • esc back • q quit"
		case jobFailed:
			help = "r retry • m open magnet instead • o page • esc back • q quit"
		default:
			help = "m open magnet instead • o page • esc back (keeps fetching) • q quit"
		}
	case m.view == savedView:
		help = "↑/↓ select • enter details • x remove • f folder • tab search • q quit"
	case m.input.Focused():
		help = "enter search • esc/↓ results • tab saved • ctrl+c quit"
	default:
		help = "enter save .torrent • m magnet • o page • c a d s filters • / search • tab saved • q quit"
	}
	return []string{
		"",
		m.details(),
		statusStyle.Render(fit(status, m.width)),
		helpStyle.Render(fit(help, m.width)),
	}
}

// fit truncates s to w display cells, then pads it to exactly w.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "…")
	if pad := w - ansi.StringWidth(s); pad > 0 {
		s += strings.Repeat(" ", pad)
	}
	return s
}

func truncate(s string, n int) string {
	return ansi.Truncate(s, n, "…")
}
