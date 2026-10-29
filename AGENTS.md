# AGENTS.md

Guidance for AI coding agents working on torrenty: a Go TUI (Bubble Tea) for searching movies and series and saving their `.torrent` files.

## Commands

```sh
go build -o torrenty.exe ./cmd/torrenty   # build
go run ./cmd/torrenty                     # run (needs a real terminal)
go vet ./...                              # lint
gofmt -w .                                # format
```

A Claude Code Stop hook (`.claude/settings.json`) runs `packaging/windows/install.ps1` after every turn. It rebuilds and reinstalls torrenty only when the source changed. Other agents should run the script themselves after changing code: `powershell -File packaging/windows/install.ps1`.

Go 1.26.5 is installed locally. Don't change the `go` directive in `go.mod`, and set `GOTOOLCHAIN=local` so Go never downloads a toolchain.

## Layout

- `cmd/torrenty/main.go`: flags only; it calls `internal.Run`.
- `internal/`: **one flat package. Do not create sub-folders or sub-packages.** Organise code by file name instead (see README "Project layout").
- `docs/`: screenshots used by the README.

## Architecture

- **Search** (`apibay.go`) implements the `Provider` interface (`result.go`). It queries `cat=200` (video) and keeps only the movie and TV sub-categories in `classify.go`. Anything NSFW is dropped.
- **Classification** (`classify.go`) runs regexes over release names and produces a `Meta`: kind (movie, episode, season pack, multi-season pack), show title, season and episode numbers, and resolution.
- **Grouping** (`group.go`): `buildRows` applies `Filters` and returns a flat `[]row` of headings and results that the list view renders. The defaults are ≥720p, seeded only, smallest first.
- **Fetching** (`fetch.go`) runs `aria2c --bt-metadata-only --bt-save-metadata` and renames `<infohash>.torrent` to the release name. aria2c is found on PATH or in winget's package folder.
- **History** (`history.go`) keeps a JSON file in `os.UserConfigDir()/torrenty`. Entries whose file is gone are dropped on load.
- **UI**: state and key handling are in `ui.go`, rendering and styles in `view.go`. The views are search, saved, and focus (one torrent only, including the replace/cancel confirm).

## Rules

- **Never write to stdout or stderr while the TUI runs.** Child processes must use buffers, and logging is discarded.
- Mutate model state only inside `Update`. Background work returns a `tea.Msg`.
- Keep dependencies minimal: standard library plus Charmbracelet. Don't add a torrent engine; aria2 does the network work.
- The project is "tiny" on purpose, so prefer the simplest change.
- Don't add tests unless asked.
