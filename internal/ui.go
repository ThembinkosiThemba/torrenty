package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type view int

const (
	searchView view = iota
	savedView
	focusView // one torrent, everything else hidden
)

type jobState int

const (
	jobRunning jobState = iota
	jobDone
	jobFailed
)

// job is one .torrent file being fetched, or already saved.
type job struct {
	r       Result
	state   jobState
	path    string
	err     error
	started time.Time
	took    time.Duration
	savedAt time.Time
	confirm bool // already saved; waiting for the user to replace or cancel
}

type (
	searchDoneMsg struct {
		query   string
		results []Result
		err     error
	}
	jobDoneMsg struct {
		j    *job
		path string
		err  error
	}
	statusMsg struct {
		text string
		err  bool
	}
)

type model struct {
	ctx      context.Context // cancelled on quit; kills running aria2c
	provider Provider
	fetcher  *Fetcher
	hist     *history // nil if the config dir is unavailable

	view      view
	backTo    view // where esc leaves the focus view to
	input     textinput.Model
	spinner   spinner.Model
	searching bool
	query     string

	results []Result
	filters Filters
	rows    []row
	kept    int
	cursor  int // index into rows; always a selectable row when any exist
	offset  int // first visible row

	jobs      []*job
	jobCursor int
	focused   *job

	status    string
	statusErr bool

	width, height int
}

func newModel(ctx context.Context, p Provider, f *Fetcher, h *history, saved []historyEntry) model {
	in := textinput.New()
	in.Placeholder = "search movies & series…"
	in.Prompt = "❯ "
	in.PromptStyle = lipgloss.NewStyle().Foreground(accent)
	in.CharLimit = 200
	in.Focus()

	m := model{
		ctx:      ctx,
		provider: p,
		fetcher:  f,
		hist:     h,
		input:    in,
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent))),
		status:   "type a query and press enter",
	}
	for _, e := range saved {
		m.jobs = append(m.jobs, &job{r: e.Result, state: jobDone, path: e.Path, savedAt: e.SavedAt})
	}
	if !f.Available() {
		m.setStatus("aria2c not found — saving .torrent files is disabled (winget install aria2.aria2)", true)
	}
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m *model) setStatus(s string, isErr bool) {
	m.status, m.statusErr = s, isErr
}

// busy reports whether anything is in flight, which keeps the spinner going.
func (m model) busy() bool {
	if m.searching {
		return true
	}
	for _, j := range m.jobs {
		if j.state == jobRunning {
			return true
		}
	}
	return false
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.input.Width = max(m.width-6, 10)
		m.scrollToCursor()
		return m, nil

	case spinner.TickMsg:
		if !m.busy() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case searchDoneMsg:
		m.searching = false
		if msg.err != nil {
			m.setStatus("search failed: "+msg.err.Error(), true)
			return m, nil
		}
		m.query, m.results = msg.query, msg.results
		m.rebuild(nil)
		m.reportCounts()
		if m.kept > 0 {
			m.input.Blur()
		}
		return m, nil

	case jobDoneMsg:
		j := msg.j
		j.took = time.Since(j.started)
		if msg.err != nil {
			j.state, j.err = jobFailed, msg.err
			m.setStatus(truncate(j.r.Name, 40)+": "+msg.err.Error(), true)
		} else {
			j.state, j.path, j.savedAt = jobDone, msg.path, time.Now()
			m.setStatus("saved "+msg.path, false)
			m.persist()
		}
		return m, nil

	case statusMsg:
		m.setStatus(msg.text, msg.err)
		return m, nil

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.view {
	case focusView:
		return m.handleFocusKey(msg)
	case savedView:
		return m.handleSavedKey(msg)
	}
	if m.input.Focused() {
		return m.handleInputKey(msg)
	}
	return m.handleListKey(msg)
}

func (m model) handleInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		q := strings.TrimSpace(m.input.Value())
		if q == "" || m.searching {
			return m, nil
		}
		m.searching = true
		m.setStatus("searching "+m.provider.Name()+"…", false)
		return m, tea.Batch(m.search(q), m.spinner.Tick)
	case "esc", "down":
		if m.kept > 0 {
			m.input.Blur()
		}
		return m, nil
	case "tab":
		m.view = savedView
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) handleListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "tab":
		m.view = savedView
	case "/", "esc":
		m.input.Focus()
		m.input.CursorEnd()
		return m, textinput.Blink
	case "up", "k":
		if !m.moveCursor(-1) {
			m.input.Focus()
			return m, textinput.Blink
		}
	case "down", "j":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-m.listHeight())
	case "pgdown":
		m.moveCursor(m.listHeight())
	case "home", "g":
		m.moveCursor(-len(m.rows))
	case "end", "G":
		m.moveCursor(len(m.rows))
	case "c":
		m.filters.Type = (m.filters.Type + 1) % typeFilters
		m.refilter()
	case "a":
		m.filters.AnyQuality = !m.filters.AnyQuality
		m.refilter()
	case "d":
		m.filters.IncludeDead = !m.filters.IncludeDead
		m.refilter()
	case "s":
		m.filters.Sort = (m.filters.Sort + 1) % sortModes
		m.refilter()
	case "enter", "t":
		if r := m.selected(); r != nil {
			return m.saveTorrent(*r)
		}
	case "m":
		if r := m.selected(); r != nil {
			return m, open(r.Magnet(), "opened magnet in your torrent app")
		}
	case "o":
		if r := m.selected(); r != nil {
			return m, open(r.PageURL(), "opened "+r.PageURL())
		}
	}
	return m, nil
}

func (m model) handleSavedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "tab", "esc":
		m.view = searchView
	case "up", "k":
		m.jobCursor = max(m.jobCursor-1, 0)
	case "down", "j":
		m.jobCursor = max(min(m.jobCursor+1, len(m.jobs)-1), 0)
	case "enter":
		if j := m.currentJob(); j != nil {
			m.openFocus(j)
		}
	case "x", "delete":
		if j := m.currentJob(); j != nil && j.state != jobRunning {
			m.jobs = append(m.jobs[:m.jobCursor], m.jobs[m.jobCursor+1:]...)
			m.jobCursor = max(min(m.jobCursor, len(m.jobs)-1), 0)
			m.persist()
		}
	case "f":
		return m, open(m.fetcher.dir, "opened "+m.fetcher.dir)
	}
	return m, nil
}

func (m model) handleFocusKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	j := m.focused
	if j.confirm {
		switch msg.String() {
		case "r", "y":
			j.confirm = false
			if !m.hasJob(j) {
				m.jobs = append(m.jobs, j)
			}
			return m, m.start(j)
		case "esc", "n", "q", "backspace":
			j.confirm = false
			m.view, m.focused = m.backTo, nil
			m.setStatus("kept existing "+j.path, false)
		case "enter":
			return m, open(j.path, "opened "+j.path)
		case "f":
			return m, open(m.fetcher.dir, "opened "+m.fetcher.dir)
		}
		return m, nil
	}
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "backspace", "left", "h":
		m.view, m.focused = m.backTo, nil
	case "enter":
		if j.state == jobDone {
			return m, open(j.path, "opened "+j.path)
		}
	case "r":
		if j.state == jobFailed {
			return m, m.start(j)
		}
	case "m":
		return m, open(j.r.Magnet(), "opened magnet in your torrent app")
	case "o":
		return m, open(j.r.PageURL(), "opened "+j.r.PageURL())
	case "f":
		return m, open(m.fetcher.dir, "opened "+m.fetcher.dir)
	}
	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.view != searchView || msg.Action != tea.MouseActionPress {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveCursor(-3)
	case tea.MouseButtonWheelDown:
		m.moveCursor(3)
	case tea.MouseButtonLeft:
		if msg.Y == 2 { // the search box
			m.input.Focus()
			return m, textinput.Blink
		}
		i := m.offset + msg.Y - listTop
		if msg.Y < listTop || i >= len(m.rows) || i >= m.offset+m.listHeight() || !m.rows[i].selectable() {
			return m, nil
		}
		m.input.Blur()
		if i == m.cursor { // second click on the same row saves it
			return m.saveTorrent(*m.rows[i].r)
		}
		m.cursor = i
	}
	return m, nil
}

// --- results list ---

// rebuild regroups results with the current filters, keeping keep selected
// if it is still visible.
func (m *model) rebuild(keep *Result) {
	m.rows, m.kept = buildRows(m.results, m.filters)
	m.cursor, m.offset = -1, 0
	for i, rw := range m.rows {
		if rw.r == nil {
			continue
		}
		if m.cursor < 0 || rw.r == keep {
			m.cursor = i
		}
		if rw.r == keep {
			break
		}
	}
	m.scrollToCursor()
}

func (m *model) refilter() {
	m.rebuild(m.selected())
	m.reportCounts()
}

func (m *model) reportCounts() {
	hidden := len(m.results) - m.kept
	switch {
	case len(m.results) == 0:
		m.setStatus(fmt.Sprintf("no results for %q", m.query), false)
	case m.kept == 0:
		m.setStatus(fmt.Sprintf("all %d results hidden by filters — press a (any quality) or d (dead)", hidden), false)
	case hidden > 0:
		m.setStatus(fmt.Sprintf("%d results for %q (%d hidden by filters)", m.kept, m.query, hidden), false)
	default:
		m.setStatus(fmt.Sprintf("%d results for %q", m.kept, m.query), false)
	}
}

// moveCursor moves by delta selectable rows, clamping at the ends. It
// reports false if it was already at the top and asked to go up.
func (m *model) moveCursor(delta int) bool {
	if m.cursor < 0 {
		return delta >= 0
	}
	step := 1
	if delta < 0 {
		step, delta = -1, -delta
	}
	moved := false
	for ; delta > 0; delta-- {
		next := -1
		for i := m.cursor + step; i >= 0 && i < len(m.rows); i += step {
			if m.rows[i].selectable() {
				next = i
				break
			}
		}
		if next < 0 {
			break
		}
		m.cursor, moved = next, true
	}
	m.scrollToCursor()
	return moved || step > 0
}

func (m *model) scrollToCursor() {
	h := m.listHeight()
	if m.cursor < 0 {
		m.offset = 0
		return
	}
	// Keep the headings directly above the cursor in view.
	top := m.cursor
	for top > 0 && !m.rows[top-1].selectable() {
		top--
	}
	if top < m.offset {
		m.offset = top
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
	m.offset = max(min(m.offset, len(m.rows)-h), 0)
}

func (m model) listHeight() int {
	return max(m.height-listTop-footerLines, 3)
}

func (m model) selected() *Result {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return nil
	}
	return m.rows[m.cursor].r
}

// --- fetching ---

func (m model) saveTorrent(r Result) (tea.Model, tea.Cmd) {
	for _, j := range m.jobs {
		if j.r.InfoHash != r.InfoHash {
			continue
		}
		m.openFocus(j)
		switch j.state {
		case jobDone:
			j.confirm = true // ask before replacing
			return m, nil
		case jobFailed:
			return m, m.start(j) // try again
		}
		return m, nil // already fetching: just show it
	}

	j := &job{r: r}
	if path := m.fetcher.targetPath(r); fileExists(path) {
		// Saved in an earlier session or by hand; ask before overwriting.
		j.state, j.path, j.confirm = jobDone, path, true
		m.openFocus(j)
		return m, nil
	}
	m.jobs = append(m.jobs, j)
	m.openFocus(j)
	if !m.fetcher.Available() {
		j.state, j.err = jobFailed, fmt.Errorf("aria2c not found — install it with: winget install aria2.aria2")
		return m, nil
	}
	return m, m.start(j)
}

func (m *model) openFocus(j *job) {
	if m.view != focusView {
		m.backTo = m.view
	}
	m.view, m.focused = focusView, j
	m.input.Blur()
}

func (m model) start(j *job) tea.Cmd {
	j.state, j.err, j.started = jobRunning, nil, time.Now()
	fetch := func() tea.Msg {
		path, err := m.fetcher.SaveTorrent(m.ctx, j.r)
		return jobDoneMsg{j: j, path: path, err: err}
	}
	return tea.Batch(fetch, m.spinner.Tick)
}

func (m model) search(q string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer cancel()
		res, err := m.provider.Search(ctx, q)
		return searchDoneMsg{query: q, results: res, err: err}
	}
}

func open(target, okText string) tea.Cmd {
	return func() tea.Msg {
		if err := openTarget(target); err != nil {
			return statusMsg{text: "could not open: " + err.Error(), err: true}
		}
		return statusMsg{text: okText}
	}
}

func (m model) hasJob(j *job) bool {
	for _, x := range m.jobs {
		if x == j {
			return true
		}
	}
	return false
}

// persist writes all saved jobs to the history file.
func (m *model) persist() {
	if m.hist == nil {
		return
	}
	var entries []historyEntry
	for _, j := range m.jobs {
		if j.state == jobDone {
			entries = append(entries, historyEntry{Result: j.r, Path: j.path, SavedAt: j.savedAt})
		}
	}
	if err := m.hist.save(entries); err != nil {
		m.setStatus("could not save history: "+err.Error(), true)
	}
}

func (m model) currentJob() *job {
	if m.jobCursor < 0 || m.jobCursor >= len(m.jobs) {
		return nil
	}
	return m.jobs[m.jobCursor]
}
