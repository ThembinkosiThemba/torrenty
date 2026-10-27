package internal

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// historyEntry is one saved .torrent file, remembered across sessions.
type historyEntry struct {
	Result  Result    `json:"result"`
	Path    string    `json:"path"`
	SavedAt time.Time `json:"saved_at"`
}

// history persists saved torrents as JSON in the user's config directory
// (%AppData%\torrenty on Windows, ~/.config/torrenty on Linux).
type history struct {
	path string
}

func openHistory() (*history, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	return &history{path: filepath.Join(dir, "torrenty", "history.json")}, nil
}

// load returns saved entries, skipping any whose .torrent file was deleted.
func (h *history) load() ([]historyEntry, error) {
	data, err := os.ReadFile(h.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []historyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	kept := entries[:0]
	for _, e := range entries {
		if fileExists(e.Path) {
			kept = append(kept, e)
		}
	}
	return kept, nil
}

// save replaces the stored history. It writes a temp file and renames it so
// a crash mid-write can't corrupt the existing history.
func (h *history) save(entries []historyEntry) error {
	if err := os.MkdirAll(filepath.Dir(h.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp := h.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, h.path)
}
