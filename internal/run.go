package internal

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI, saving .torrent files into dir. It blocks until quit.
func Run(dir string) error {
	// Cancelled on exit so any running aria2c processes are killed.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// History is a nicety: without a config dir, run without it.
	var saved []historyEntry
	hist, err := openHistory()
	if err == nil {
		saved, err = hist.load()
	}
	if err != nil {
		hist = nil
	}

	m := newModel(ctx, newApibay(), newFetcher(dir), hist, saved)
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}
