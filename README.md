# berichtsheft-cli

CLI + TUI for the [berichtsheft](https://github.com/veloben/berichtsheft) API.

## Features (MVP)

- `year <YYYY>`: show week/day summary for a year
- `day get <YYYY> <WW> <D>`: print a day JSON
- `day set ...`: patch/save a day from terminal
- `tui`: interactive weekly view/editor (Bubble Tea)

Includes compatibility normalization for legacy values:

- `school -> schule` / `schulzeit`
- `holiday -> urlaub`
- `other -> sonstiges`

## Install

```bash
go install github.com/veloben/berichtsheft-cli/cmd/berichtsheft-cli@latest
```

Or local build:

```bash
go build ./cmd/berichtsheft-cli
```

## Usage

Default API base URL: `http://127.0.0.1:3847`

Override with flag or env:

```bash
export BERICHTSHEFT_BASE_URL="http://127.0.0.1:8080"
# or --base-url ...
```

### Year overview

```bash
berichtsheft-cli year 2026
```

### Get one day

```bash
berichtsheft-cli day get 2026 11 2
```

### Set one day

```bash
berichtsheft-cli day set --status anwesend --location betrieb --time 8 --text "Heute API Client gebaut" 2026 11 2
```

> `--time` is integer-only (`0..12`), matching the current backend schema.

### Start TUI

```bash
berichtsheft-cli tui --year 2026 --week 11
```

TUI keys:

- `j/k` or `↑/↓` select day
- `h/l` or `←/→` previous/next week
- `s` cycle status
- `o` toggle location (Ort)
- `+/-` adjust hours
- `e` or `Enter` open text editor (`Ctrl+S` save text to form, `Esc` cancel)
- `w` save selected day
- `a` save all changed days
- `r` reload week
- `?` toggle shortcut help
- `q` quit

## Notes

- Validation mirrors frontend rules (e.g. non-empty text except `urlaub`, time clamped `0..12`).
- This tool talks to the same REST endpoints as the web app.
