//go:build ignore

// Parked: the Bubble Tea TUI. Needs adapting to the blocking Downloader API
// in download.go before it can be re-enabled.

package main

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const tickInterval = time.Second

var (
	accent = lipgloss.Color("#7D56F4")
	subtle = lipgloss.Color("241")

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(accent).Padding(0, 1)
	tabStyle    = lipgloss.NewStyle().Foreground(subtle).Padding(0, 1)
	activeTab   = lipgloss.NewStyle().Foreground(accent).Bold(true).Underline(true).Padding(0, 1)
	helpStyle   = lipgloss.NewStyle().Foreground(subtle)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dimStyle    = lipgloss.NewStyle().Foreground(subtle)
	nameStyle   = lipgloss.NewStyle().Bold(true)
	selectedBar = lipgloss.NewStyle().Foreground(accent).Bold(true)
)

type view int

const (
	searchView view = iota
	downloadsView
)

type (
	searchDoneMsg struct {
		query   string
		results []Result
		err     error
	}
	tickMsg   time.Time
	statusMsg struct {
		text string
		err  bool
	}
)

type model struct {
	provider Provider
	dl       *Downloader

	view     view
	input    textinput.Model
	table    table.Model
	spinner  spinner.Model
	bar      progress.Model
	results  []Result
	query    string
	busy     bool
	dlCursor int

	status    string
	statusErr bool
	lastTick  time.Time

	width, height int
}

func newModel(p Provider, d *Downloader) model {
	in := textinput.New()
	in.Placeholder = "search torrents…"
	in.Prompt = "❯ "
	in.PromptStyle = lipgloss.NewStyle().Foreground(accent)
	in.CharLimit = 200
	in.Focus()

	t := table.New(table.WithFocused(false))
	ts := table.DefaultStyles()
	ts.Header = ts.Header.BorderStyle(lipgloss.NormalBorder()).BorderBottom(true).BorderForeground(subtle).Bold(true)
	ts.Selected = ts.Selected.Foreground(lipgloss.Color("#FAFAFA")).Background(accent).Bold(false)
	t.SetStyles(ts)

	sp := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(lipgloss.NewStyle().Foreground(accent)))

	return model{
		provider: p,
		dl:       d,
		input:    in,
		table:    t,
		spinner:  sp,
		bar:      progress.New(progress.WithDefaultGradient(), progress.WithoutPercentage()),
		lastTick: time.Now(),
		status:   "type a query and press enter",
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, tick())
}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) search(q string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		res, err := m.provider.Search(ctx, q)
		return searchDoneMsg{query: q, results: res, err: err}
	}
}

func (m *model) setStatus(s string, isErr bool) {
	m.status, m.statusErr = s, isErr
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tickMsg:
		now := time.Time(msg)
		m.dl.Tick(now.Sub(m.lastTick))
		m.lastTick = now
		return m, tick()

	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case searchDoneMsg:
		m.busy = false
		if msg.err != nil {
			m.setStatus("search failed: "+msg.err.Error(), true)
			return m, nil
		}
		m.query, m.results = msg.query, msg.results
		m.table.SetRows(m.rows())
		m.table.GotoTop()
		if len(m.results) == 0 {
			m.setStatus(fmt.Sprintf("no results for %q", msg.query), false)
			return m, nil
		}
		m.setStatus(fmt.Sprintf("%d results for %q", len(m.results), msg.query), false)
		m.focusTable()
		return m, nil

	case metadataMsg:
		m.dl.OnMetadata(msg)
		switch dl := msg.dl; dl.State {
		case stateSaved:
			m.setStatus("saved "+dl.Path, false)
		case stateFailed:
			m.setStatus(dl.Name+": "+dl.Err.Error(), true)
		}
		return m, nil

	case statusMsg:
		m.setStatus(msg.text, msg.err)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "tab", "shift+tab":
		if m.view == searchView {
			m.view = downloadsView
			m.input.Blur()
			m.table.Blur()
		} else {
			m.view = searchView
			m.focusTable()
		}
		return m, nil
	}

	if m.view == downloadsView {
		return m.handleDownloadsKey(msg)
	}

	// Typing in the search box.
	if m.input.Focused() {
		switch msg.String() {
		case "enter":
			q := strings.TrimSpace(m.input.Value())
			if q == "" || m.busy {
				return m, nil
			}
			m.busy = true
			m.setStatus("searching "+m.provider.Name()+"…", false)
			return m, tea.Batch(m.search(q), m.spinner.Tick)
		case "esc", "down":
			if len(m.results) > 0 {
				m.focusTable()
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	// Navigating results.
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "/", "s", "esc":
		m.table.Blur()
		m.input.Focus()
		m.input.CursorEnd()
		return m, textinput.Blink
	case "enter", "d":
		return m.startDownload(false)
	case "t":
		return m.startDownload(true)
	case "m":
		if r, ok := m.selected(); ok {
			return m, openCmd(r.Magnet(), "opened magnet in default app")
		}
		return m, nil
	case "up", "k":
		if m.table.Cursor() == 0 {
			m.table.Blur()
			m.input.Focus()
			return m, textinput.Blink
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m model) handleDownloadsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.dl.Items)
	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "up", "k":
		if m.dlCursor > 0 {
			m.dlCursor--
		}
	case "down", "j":
		if m.dlCursor < n-1 {
			m.dlCursor++
		}
	case "x", "delete", "backspace":
		if n == 0 {
			return m, nil
		}
		name := m.dl.Items[m.dlCursor].Name
		m.dl.Remove(m.dlCursor)
		if m.dlCursor >= len(m.dl.Items) && m.dlCursor > 0 {
			m.dlCursor--
		}
		m.setStatus("removed "+name+" (files kept on disk)", false)
	case "o":
		return m, openCmd(m.dl.dir, "opened "+m.dl.dir)
	}
	return m, nil
}

func (m model) startDownload(saveOnly bool) (tea.Model, tea.Cmd) {
	r, ok := m.selected()
	if !ok {
		return m, nil
	}
	_, cmd, err := m.dl.Add(r, saveOnly)
	if err != nil {
		m.setStatus(err.Error(), true)
		return m, nil
	}
	verb := "downloading"
	if saveOnly {
		verb = "fetching .torrent for"
	}
	m.setStatus(fmt.Sprintf("%s %s — tab to view downloads", verb, r.Name), false)
	return m, cmd
}

func (m model) selected() (Result, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.results) {
		return Result{}, false
	}
	return m.results[i], true
}

func (m *model) focusTable() {
	if len(m.results) == 0 {
		m.input.Focus()
		return
	}
	m.input.Blur()
	m.table.Focus()
}

// layout sizes the table to the terminal. Fixed chrome: header (2), input (2),
// status + help (3).
func (m *model) layout() {
	w := max(m.width-2, 40)
	sizeW, seedW, leechW, addedW := 10, 6, 6, 10
	nameW := max(w-sizeW-seedW-leechW-addedW-10, 10)
	m.table.SetColumns([]table.Column{
		{Title: "Name", Width: nameW},
		{Title: "Size", Width: sizeW},
		{Title: "Seed", Width: seedW},
		{Title: "Leech", Width: leechW},
		{Title: "Added", Width: addedW},
	})
	m.table.SetWidth(w)
	m.table.SetHeight(max(m.height-8, 3))
	m.table.SetRows(m.rows())
	m.input.Width = w - 4
	m.bar.Width = min(max(w-30, 10), 50)
}

func (m model) rows() []table.Row {
	rows := make([]table.Row, len(m.results))
	for i, r := range m.results {
		rows[i] = table.Row{
			r.Name,
			humanBytes(r.Size),
			fmt.Sprint(r.Seeders),
			fmt.Sprint(r.Leechers),
			r.Added.Format("2006-01-02"),
		}
	}
	return rows
}

func (m model) View() string {
	if m.width == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(m.header())
	b.WriteString("\n\n")

	if m.view == searchView {
		b.WriteString(m.searchBody())
	} else {
		b.WriteString(m.downloadsBody())
	}

	// Pin status and help to the bottom.
	footer := m.footer()
	body := b.String()
	gap := m.height - lipgloss.Height(body) - lipgloss.Height(footer)
	if gap > 0 {
		body += strings.Repeat("\n", gap)
	}
	return body + footer
}

func (m model) header() string {
	searchTab, dlTab := tabStyle, tabStyle
	if m.view == searchView {
		searchTab = activeTab
	} else {
		dlTab = activeTab
	}
	dlLabel := "Downloads"
	if n := len(m.dl.Items); n > 0 {
		dlLabel = fmt.Sprintf("Downloads (%d)", n)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		titleStyle.Render("torrenty"), " ",
		searchTab.Render("Search"),
		dlTab.Render(dlLabel),
	)
}

func (m model) searchBody() string {
	return m.input.View() + "\n\n" + m.table.View()
}

func (m model) downloadsBody() string {
	if len(m.dl.Items) == 0 {
		return dimStyle.Render("  nothing yet — pick a result and press enter to download")
	}
	var b strings.Builder
	for i, dl := range m.dl.Items {
		cursor := "  "
		name := nameStyle.Render(truncate(dl.Name, m.width-6))
		if i == m.dlCursor {
			cursor = selectedBar.Render("▌ ")
		}
		b.WriteString(cursor + name + "\n")
		b.WriteString("  " + m.downloadLine(dl) + "\n\n")
	}
	return b.String()
}

func (m model) downloadLine(dl *Download) string {
	switch dl.State {
	case stateMetadata:
		return dimStyle.Render(fmt.Sprintf("%s %s… %d peers", m.spinnerFrame(), dl.State, dl.Peers))
	case stateDownloading:
		return fmt.Sprintf("%s %5.1f%%  %s / %s  %s/s  %d peers",
			m.bar.ViewAs(dl.Progress()), dl.Progress()*100,
			humanBytes(dl.Done), humanBytes(dl.Total),
			humanBytes(int64(dl.Speed)), dl.Peers)
	case stateDone:
		return m.bar.ViewAs(1) + okStyle.Render(" ✓ complete  ") + dimStyle.Render(dl.Path)
	case stateSaved:
		return okStyle.Render("✓ saved ") + dimStyle.Render(dl.Path)
	case stateFailed:
		return errStyle.Render("✗ " + dl.Err.Error())
	}
	return ""
}

func (m model) spinnerFrame() string {
	// The spinner only animates while searching; show a static dot otherwise.
	if m.busy {
		return m.spinner.View()
	}
	return "•"
}

func (m model) footer() string {
	status := m.status
	if m.busy {
		status = m.spinner.View() + " " + status
	}
	statusStyle := dimStyle
	if m.statusErr {
		statusStyle = errStyle
	}

	var help string
	switch {
	case m.view == downloadsView:
		help = "↑/↓ select • x remove • o open folder • tab search • q quit"
	case m.input.Focused():
		help = "enter search • esc/↓ results • tab downloads • ctrl+c quit"
	default:
		help = "enter download • t save .torrent • m open magnet • / search • tab downloads • q quit"
	}
	if n := m.dl.Active(); n > 0 {
		help = fmt.Sprintf("%d active • ", n) + help
	}
	return statusStyle.Render(truncate(status, m.width-1)) + "\n" + helpStyle.Render(truncate(help, m.width-1))
}

func truncate(s string, n int) string {
	if n <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// openCmd opens a URL, magnet link or folder with the OS default handler.
func openCmd(target, okText string) tea.Cmd {
	return func() tea.Msg {
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
			return statusMsg{text: "could not open: " + err.Error(), err: true}
		}
		go cmd.Wait()
		return statusMsg{text: okText}
	}
}
