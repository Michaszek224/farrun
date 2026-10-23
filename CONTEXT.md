# FarRun: context for whoever works on this next

Read this first. It is the memory of the project: what it is, how it fits together, why things
are the way they are, what went wrong before, and what comes next. `README.md` is the
user-facing description; this file is for people (and AI agents) changing the code.

## 1. What this is

A personal tool to **track and analyze my runs**. It reads Garmin `.tcx` exports, stores them in
SQLite, and serves a local web UI: a filterable and sortable run list, a detail page with
charts, and a page that compares runs. Written in Go, by someone **learning Go**, so code should
stay idiomatic, readable and commented where the reason is not obvious.

**Status:** usable end to end. The next planned feature is **showing runs on maps** (section 11).

## 2. Running it

```
go run ./cmd/farrun import            # read activities/*.tcx into farrun.db
go run ./cmd/farrun serve             # web UI on http://127.0.0.1:8080
go run ./cmd/farrun import --anon     # synthetic demo data (anonActivities/ -> anon.db)
go run ./cmd/farrun serve  --anon
go test -race ./...
```

Flags (both commands): `--db PATH`, `--anon`, `--tz ZONE` (default `Europe/Warsaw`). `serve` also
has `--addr` (default `127.0.0.1:8080`; a warning is printed if it is not loopback).
`--anon` defaults to its own database (`anon.db`) so demo data can never land in the real one.

Needs Go 1.27+. The only dependency is `modernc.org/sqlite` (pure Go, no CGO). Chart.js 4.4.7
and JetBrains Mono are vendored into `internal/web/static/`; there are no other assets.

## 3. Map of the code

```
cmd/farrun/            CLI: `import` and `serve` commands, flags, graceful shutdown
internal/model/        Plain types: Activity, Lap, Point, Position. No logic, no imports.
internal/source/       .tcx XML parser -> model types (bad files are skipped, not fatal)
internal/importer/     Run(): parse a folder, save it, report. Used by `import`.
internal/store/        SQLite: open, migrations, SaveActivities, ListActivities, Activity(id)
internal/analysis/     Pure functions: pace, avg/max HR, elevation gain, chart Series
internal/query/        The SQL-like filter language for the run list (pure, fuzzed)
internal/web/          HTTP handlers, html/template pages, static files (all //go:embed)
```

Dependency direction: `cmd -> web, importer -> store, source, query, analysis -> model`.
`model`, `analysis` and `query` import nothing from the rest of the app, which is why they are
easy to test. Keep it that way.

Data flow: `.tcx` -> `source` -> `model.Activity` -> `store` (SQLite) -> `web` handlers ->
`analysis`/`query` -> templates and JSON -> the browser (`charts.js`, `sort.js`, ...).

## 4. Data and the database

- **Tables:** `activities`, `laps`, `trackpoints` (see `internal/store/migrations/`). Trackpoints
  hold `latitude`, `longitude`, altitude, cumulative distance, speed, heart rate, cadence; every
  one of them is **nullable** (NULL = the watch recorded nothing).
- **Optional values are pointers** in `model` (`*float64`, `*int`) and nil means "no data". Never
  store a fake `0` for missing data: it ruins averages and chart axes.
- **Timestamps** are Unix milliseconds in UTC. Convert to a time zone only when displaying
  (`--tz`). A run at 23:30 UTC on 31 Dec is 1 Jan in Warsaw; the query language and the date
  columns follow the display zone on purpose.
- **Migrations** use `PRAGMA user_version` and the numbered files in `internal/store/migrations`.
  To change the schema, add `003_*.sql`; never edit an applied one. A database from a newer
  farrun is refused. Migration 001 uses `IF NOT EXISTS` so it can adopt databases that predate
  migrations.
- **Import is idempotent and atomic.** Activities are keyed by `(source, external_id)`, where
  the id is the start time in RFC 3339. Re-importing adds nothing; the first version of an
  activity wins (no update path yet). The whole import is one transaction, validated up front.
- **SQLite settings** live in the DSN (`store.dsn`): WAL, `foreign_keys(1)`, `busy_timeout`. They
  are per connection, so they must stay in the DSN, not in a one-off `PRAGMA`. The pool is
  `SetMaxOpenConns(1)`; inside a transaction use only `tx`, never `s.db`, or it deadlocks.
- All driver-specific code is in `store.Open`/`store.dsn`; switching to `mattn/go-sqlite3` would
  mean the driver name (`sqlite3`), the DSN pragmas, and nothing else. Not planned: modernc
  avoids needing a C compiler.

## 5. The web UI

Server-rendered `html/template` pages plus small vanilla-JS enhancements. No Node, no build step.

| Route | What |
|---|---|
| `GET /` | redirects to `/activities` |
| `GET /activities[?q=...]` | run list: filter box, sortable columns, tick runs to compare |
| `GET /activities/{id}` | stats, laps table, charts of pace / heart rate / altitude / cadence |
| `GET /compare?id=1&id=2...` | 2 to 10 runs, one table, charts overlaid by distance |
| `GET /api/activities/{id}/series[?points=N]` | chart data as JSON (default 500, max 2000) |
| `GET /static/...` | embedded assets |

- **Sorting** (`sort.js`) is client-side on raw `data-sort` values in each cell. A column starts
  in its "best first" direction (newest, longest, fastest, A-Z); clicking again reverses it;
  clicking another column starts that one over. The sort is kept in `sessionStorage` because the
  filter reloads the page. Rows without a value (a pace needs a distance) sort last.
- **Filtering** is server-side (`internal/query`, `?q=`), so queries are bookmarkable and
  testable in Go. Syntax: bare words search date text, sport and device; `field op value`
  with `distance`, `pace`, `time`, `date`, `sport`, `device`; `and` `or` `not` `( )`,
  `between a and b`, `like`; side by side means `and`. Rules worth remembering:
  - values are compared **as the list shows them** (4996 m is "5.00 km" and matches
    `distance >= 5km`; pace is whole seconds);
  - `pace < 6:20` means *faster*; a run with no distance has no pace and matches no pace
    comparison, not even `!=` (like SQL NULL);
  - dates are whole periods in the display zone: `date = 2026` is the year,
    `date > 2026-03` is after the end of March;
  - a query that does not parse is **not applied**: the page shows all runs plus the message
    and a `^` under the problem. Never show an empty list for a typo.
  - `maxLength` 500 and `maxDepth` 20 guard against hostile input (it comes from the URL).
- **Compare** uses distance, not time, on the x axis so runs of different lengths overlay.
- **Charts** (`charts.js`) share one x range per page (`min 0`, `max = longest run`); Chart.js
  otherwise sizes each axis from the points it draws. The pace axis contains all data and
  uses tidy tick steps; heart rate and cadence use whole-number ticks.

### Look and feel

Orange cyberpunk with a terminal (TUI) flavour, **dark only**, monospace only:
JetBrains Mono (bundled, SIL OFL, licence in `static/fonts/OFL.txt`), square frames with titles
set into the border (`┤ PACE ├`), neon glow, faint scanlines, `FARRUN█` with a blinking cursor,
`[ BUTTON ]` styling. Everything is in `internal/web/static/style.css`; the palette sits in
`:root` (`--accent` orange, `--cyan` links, `--series-0..5` chart lines). Dates have a flowing
gradient, and "decrypt" from random glyphs into the real digits on load and on hover
(`dates.js`, real text kept in `aria-label`). **All motion switches off under
`prefers-reduced-motion`.** The date gradient is stepped (`steps(120)`) because 52 smoothly
animated gradients cost twice the CPU for no visible gain.

## 6. Data-quality rules (internal/analysis)

Garmin data has glitches; the charts would be unreadable without these. All constants are at the
top of `analysis.go`.

- **Pace** is only drawn when moving: speed below `minMovingSpeed` (1.0 m/s, about 16:40/km) has
  no pace. Walking (10 to 12 min/km) is real data and stays.
- **The first 5 s after any standstill** have no pace (`rampUpSeconds`): that is acceleration, not
  pace. This runs *before* the spike filter, which would otherwise delete short stops.
- **Spike filter** (Hampel-style): a cadence, heart-rate or speed sample is dropped when it is
  further from the median of its neighbours (5 each side) than both 3 scaled MADs and a floor
  (`spikeFloorCadence` 20, `spikeFloorHR` 25, `spikeFloorSpeed` 1.5). Sustained changes survive.
  Checked against all 52 real runs: it removes about 0.6% of cadence, 0.4% of pace, 0.01% of HR,
  and what it removed was real glitches (GPS speed jumps, cadence ramp-down next to a stop).
- **Cadence 0 means no data** (the watch reports 0 while standing).
- **Known limit:** a glitch longer than about 6 samples looks like a real change and is kept
  (example: 7 low cadence readings at the start of one run). Possible fix: extend the
  "after a standstill" rule to cadence with about a 10 s window.
- **Elevation gain** ignores changes under 1 m (barometric jitter).
- Charts average neighbouring points into buckets when there are more than `points` of them.

## 7. Conventions

- **Go style:** match the surrounding code: small packages, nil pointers for missing data,
  `fmt.Errorf("...: %w", err)` wrapping, sentinel errors for expected cases
  (`store.ErrNotFound`), comments that say *why*. `gofmt`, `go vet`, `go test -race ./...` must be
  clean before a commit.
- **Tests:** table-driven where it fits, `t.TempDir()`, `httptest` for handlers. Every feature
  came with tests, including edge cases and error paths. The query package also has a fuzz test
  (`go test ./internal/query -run '^$' -fuzz FuzzParse -fuzztime 30s`).
- **JavaScript is not unit-tested in the repo.** It was verified with headless Chromium driven by
  `puppeteer-core` from a scratch directory (real clicks, keyboard, reduced motion, CPU
  measurements, screenshots). If JS grows, consider adding a small test setup to the repo.
- **Commits:** short, plain messages, one per milestone. **No `Co-Authored-By` or other
  attribution lines.** Nothing is pushed unless asked.
- **Process the owner likes:** for anything big or visual (the theme, sorting, the query box), write
  down the understanding or a plan first, show a preview where possible, get a yes, then build,
  test and commit. Small, clearly specified fixes can just be done.
- **Explain as you go:** the owner is learning Go and wants to understand what changed and why.
- Update `README.md` when behaviour or commands change.

## 8. Security and privacy

- The data includes **GPS tracks of where the owner runs and lives.** `activities/` and `*.db`
  (and the `-wal`/`-shm` files) are gitignored and must never be committed.
  `anonActivities/` holds **synthetic** runs only (shifted coordinates), safe to commit.
- The server binds to `127.0.0.1` by default and warns otherwise.
- Pages are served with `Content-Security-Policy: default-src 'self'`, `nosniff` and
  `Referrer-Policy: no-referrer`. That means **no inline scripts or styles, nothing from other
  sites** (no CDN, no web fonts, no tile servers). Everything is embedded and self-hosted. Any
  new feature that wants an external request must be an explicit, off-by-default opt-in.
- `html/template` auto-escapes; the filter text is echoed back in the box and in messages and
  is covered by tests (XSS).

## 9. Gotchas and lessons from building this

- **Port 8080 was already taken** by an unrelated old Docker container (`golang_cointainer`, from
  `~/projects/golang/postgresqlTest`, host networking). It was stopped and its restart policy set
  to `no`; Postgres and Redis containers there were left alone. If `serve` says "address already
  in use", check `ss -ltnp | grep 8080` and `docker ps` before assuming it is farrun.
- **zsh:** an unquoted glob that matches nothing aborts the command (`grep --include=*.go`,
  `ls *.db*`): quote patterns. `pkill -f pattern` can match its own shell: use `pkill -x farrun`.
- **`html/template` writes `+` as `&#43;`** inside attributes (browsers decode it); tests must
  compare decoded hrefs.
- **Chart.js 4:** dotted grid lines are `scales.x.border.dash`, not `grid.borderDash`. Points outside
  a user-set y range are ignored when sizing the x axis. Canvas text does not re-render when a
  web font arrives late, so `charts.js` waits for the font.
- **Go has no built-in `.woff2` MIME type**; `server.go` registers `font/woff2`.
- **The CSP also blocks injected `<style>`/`<script>` in tests**: use `page.setBypassCSP(true)`
  in the test browser only.
- **Tests that count `tbody tr`** also count the query-help table: scope selectors to
  `[data-compare-form] tbody tr`.
- **Font swap:** the page first paints in the fallback font a pixel wider than JetBrains Mono;
  fonts are preloaded in `layout.html` to shrink that window.
- A bare `%v` of a large float prints `1e+06`; `formatNum` exists for `data-sort` values.
- Real data spans 2024 to 2026 (52 runs, one Forerunner 745). The real database is a local file
  (`farrun.db`, ignored by git). Some runs have trackpoints without GPS (first seconds, tunnels);
  treat a missing position as normal.

## 10. Git state

`main`, remote `origin` (github.com/michaszek224/farrun). **Many commits are not pushed yet** (see
`git log origin/main..HEAD`). History from the web-UI work: import/serve split, analysis package,
list/detail/compare pages, chart fixes, sorting, theme, animated dates, filter box.

## 11. Future work: showing runs on maps  (NOT implemented; planned)

**Goal:** show where each run went, on the run's page first, later on the compare page.
The owner asked for this to be written down for the future; **do not build it until asked**, and
follow the working agreement: show a preview and get a yes before implementing.

### What already exists
- Every trackpoint has `latitude`/`longitude` in the database; `store.Activity()` already returns
  them as `Point.Position` (nil = no GPS for that sample).
- `analysis.SeriesPoint` does **not** carry a position, so the JSON API has none yet.
- The synthetic runs `anonActivities/activity_anon_002..006` are loops around a fixed point, so a
  map can be developed and screenshotted without touching real data. `001` is real-derived with
  shifted coordinates.

### Recommended approach, in two phases

**Phase 1: an offline route drawing (no map tiles). Start here.**
- A `┤ ROUTE ├` panel on the run page, drawn as an **inline SVG** (the CSP allows it, and no
  network is involved, so privacy is unchanged).
- Projection: simple equirectangular, `x = lon * cos(lat0)`, `y = lat`, fitted into the panel
  with padding. Plenty accurate at the scale of a run; no library needed. Add a scale bar
  (haversine) and north-up orientation.
- Draw start and finish markers (theme colours: orange start, pink finish) and the polyline in
  `--series-0`. **Split the line at gaps** (samples with no position) and at GPS jumps (consecutive
  points implausibly far apart), the same idea as the speed spike filter.
- Optional colouring of segments by **pace or heart rate** with the existing palette.
- **Link map and charts:** hovering a chart moves a marker on the map (and the reverse), using
  distance as the shared key, which `SeriesPoint.DistanceMeters` already provides.
- Hide the panel entirely when a run has no GPS (e.g. treadmill).
- Where it lives: a pure `internal/geo` (or `analysis.Route`) package: `Route(points, maxPoints)`
  that downsamples (bucket averaging or Douglas-Peucker), drops no-GPS samples, splits segments,
  and returns lat/lon plus distance. Expose it as `GET /api/activities/{id}/route` (JSON) or
  extend the series response; the SVG can be built in the browser (`route.js`) or server-side.
  Doing the projection server-side keeps the JS tiny and testable in Go.
- Tests: projection and fit, downsampling keeps endpoints, gap and jump splitting, a run with no
  GPS, a run with a single point, handler JSON (nulls and empty arrays), then browser checks as
  for the other pages.

**Phase 2: real map tiles, strictly opt-in.**
- Vendor Leaflet next to Chart.js (same pattern, pinned version, licence file).
- Tile requests **leak the area the owner runs in to the tile server**, so this must be **off by
  default**, enabled by an explicit flag such as `--tiles URL-TEMPLATE`, and the CSP must allow
  that one host in `img-src` only when the flag is set. Respect the tile provider's usage policy
  and show the required attribution (e.g. "© OpenStreetMap contributors"). Self-hosted or
  pre-downloaded tiles would avoid the leak entirely.
- Keep the Phase 1 SVG as the default and as the fallback when tiles fail or are disabled.

### Related ideas for the map work
- **Compare page:** overlay several routes in one SVG using `--series-0..5`, same legend swatches.
- **Privacy zone:** an option to trim the first and last N metres of a route (home), always on
  for anon data and screenshots.
- **List thumbnails:** a tiny route outline per row (cheap SVG) once Phase 1 exists; mind the cost
  with 50+ rows (draw on demand, or only once visible).
- **Elevation on the map:** colour by altitude, or a profile strip under the map.
- Theme: reuse the frames, glow and fonts; the map should look like part of the terminal UI.

## 12. Other open ideas (no commitment)

- Lap split chart/table with pace per lap (laps are already stored).
- `--update` on import to refresh an already-imported activity (today the first version wins).
- More query fields: average HR, climb, laps; relative dates (`date > -30d`); saved queries.
- Personal bests, weekly and monthly totals (needs SQL aggregates in `store`).
- Pagination or virtual scrolling if the list grows into the thousands.
- A favicon (the browser console logs a harmless 404 for `/favicon.ico`).
- Extend the "after a standstill" rule to cadence (see the known limit in section 6).
- A separate read-only SQLite pool if pages ever stall during an import.
