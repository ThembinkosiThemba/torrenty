# torrenty

A tiny terminal UI for finding movies and series and saving their `.torrent` files.

![torrenty](docs/torrenty.png)

## Features

- **Movies and series only.** Searches cover video categories only; music, images, clips and NSFW are filtered out.
- **Smart series grouping.** Results are grouped by show, then by multi-season packs, seasons, season packs and episodes. Episodes are listed in episode order.
- **Quality filter.** Anything below 720p is hidden by default, and dead torrents (0 seeders) are hidden too.
- **Smallest first.** Each group starts with the smallest torrent. You can switch to most seeders or newest.
- **Focused download screen.** Choosing a torrent shows only that torrent while its `.torrent` file is fetched.
- **Persistent history.** Saved torrents are remembered between sessions. If you save one you already have, torrenty asks before replacing it.
- **Open anywhere.** You can send a magnet link straight to your torrent app or open the torrent's web page.

## Requirements

- Go 1.26+
- [aria2](https://aria2.github.io/) (`aria2c`) to fetch `.torrent` files from magnet links:

  ```sh
  winget install aria2.aria2      # Windows
  brew install aria2              # macOS
  sudo apt install aria2          # Debian/Ubuntu
  ```

  Without aria2 you can still search and open magnet links in your torrent app.

## Install & run

```sh
go run ./cmd/torrenty
# or
go build -o torrenty.exe ./cmd/torrenty
./torrenty.exe -dir D:/Torrents
```

| Flag   | Default                  | Description                          |
|--------|--------------------------|--------------------------------------|
| `-dir` | `~/Downloads/torrenty`   | Where `.torrent` files are saved     |

Saved-torrent history is stored in your config directory: `%AppData%\torrenty\history.json` on Windows, or `~/.config/torrenty/history.json` on Linux.

## Keys

**Search**

| Key | Action |
|-----|--------|
| `enter` | Search (in the box) / save `.torrent` (on a result) |
| `↑/↓`, `pgup/pgdn`, `g/G` | Move through the results |
| `m` | Open the magnet link in your torrent app |
| `o` | Open the torrent's web page |
| `c` | Switch type: movies + series / movies / series |
| `a` | Toggle quality: ≥720p / any |
| `d` | Toggle dead torrents |
| `s` | Switch sort: smallest / most seeders / newest |
| `/` | Back to the search box |
| `tab` | Go to the Saved tab |
| mouse | Click to select a row, click again to save it; the wheel scrolls |

**Download screen**

| Key | Action |
|-----|--------|
| `enter` | Open the saved `.torrent` |
| `r` | Replace an existing file or retry a failed fetch |
| `f` | Open the download folder |
| `esc` | Go back (a fetch keeps running in the background) |

**Saved**: `enter` shows details, `x` removes the entry from the history, `f` opens the folder.

## License

[MIT](LICENSE)
