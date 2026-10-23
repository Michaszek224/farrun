# FarRun

A (hopefully great, in the future) project to track and analyze my runs

## What it does

- Reads `.tcx` files exported from Garmin, stored in the `activities` folder :white_check_mark:
- Stores the runs in a SQLite database :white_check_mark:
- Shows them in a web UI: run list, run details with charts, compare runs :white_check_mark:
- Route map :x:

## Status

Early days, but usable. Import your runs once, then browse them in the browser.
The route map is next.

## Running it

Put your `.tcx` files in `activities/`, import them, then start the web UI:

    go run ./cmd/farrun import
    go run ./cmd/farrun serve

and open <http://127.0.0.1:8080>. No `.tcx` files handy? Try the anonymized demo data:

    go run ./cmd/farrun import --anon
    go run ./cmd/farrun serve --anon

Needs Go 1.27 or newer.

## The web UI

- **Activities** (`/activities`): all runs with date, distance, time and pace. Click a
  column header to sort: the first click puts the newest, longest or fastest first (A to Z
  for sport), the next one reverses it. Tick two or more runs and press *Compare selected*;
  your ticks stay while you sort.
- **Run details** (`/activities/{id}`): distance, time, average pace, heart rate,
  elevation gain, a table of laps, and charts of pace, heart rate, altitude and cadence
  over distance.
- **Compare** (`/compare?id=1&id=2`): the selected runs in one table, with their charts
  overlaid by distance. Between 2 and 10 runs.
- **JSON** (`/api/activities/{id}/series`): the chart data of one run, optionally limited
  with `?points=N` (default 500, at most 2000).

The server listens on `127.0.0.1:8080` only, so nobody else on the network can see your
runs. Change it with `--addr` (you get a warning if the address is not local). Nothing is
loaded from other sites: Chart.js 4.4.7 (MIT) and the JetBrains Mono font (SIL Open Font
License 1.1, see `internal/web/static/fonts/OFL.txt`) are bundled in `internal/web/static`.

The look is orange cyberpunk with a terminal feel: dark only, monospace, square frames with
titles in the border, neon glow and faint scanlines. It is all in
`internal/web/static/style.css`: the palette sits in `:root` at the top (`--accent`,
`--cyan`, `--series-0` to `--series-5` for chart lines), and the glow and scanlines can be
dialled down there too.

Dates have a little life of their own: a neon gradient slowly flows through them, they
"decrypt" from random glyphs into the real digits when a page loads, and again when you
point at one (`internal/web/static/dates.js`). All of it switches off if your system asks
for reduced motion.

## Flags

Shared by `import` and `serve`:

- `--db PATH`: the SQLite database. Defaults to `farrun.db`, or `anon.db` with `--anon`,
  so demo data never ends up in your real database (unless you point `--db` at it).
- `--anon`: use the anonymized demo data (`anonActivities`) and `anon.db`.
- `--tz ZONE`: time zone for showing times, default `Europe/Warsaw`
  (for example `--tz America/New_York`).

Only for `serve`: `--addr HOST:PORT`, default `127.0.0.1:8080`.

## About importing

Importing the same files again is safe: activities are matched by source and external ID,
so they are not duplicated. The whole import is one transaction, so it either fully
succeeds or leaves the database untouched. A `.tcx` file that cannot be parsed is skipped
with a warning and does not stop the others.

The schema is versioned (`PRAGMA user_version`) and upgraded automatically when the
database is opened. Migrations live in `internal/store/migrations`.

## Layout

- `cmd/farrun` - CLI entry point: the `import` and `serve` commands
- `internal/importer` - parses a folder of `.tcx` files and saves them
- `internal/model` - domain types
- `internal/source` - `.tcx` parser
- `internal/store` - SQLite persistence and migrations
- `internal/analysis` - pace, heart rate, elevation gain and chart series (pure functions)
- `internal/web` - HTTP handlers, HTML templates and static files (embedded in the binary)

## Development

    go test -race ./...

## Notes

The `activities` folder is gitignored, because the files contain GPS tracks.
The local databases (`*.db`, along with their `-wal`/`-shm` files) are ignored too.
`anonActivities` holds synthetic runs only; they are safe to commit.
