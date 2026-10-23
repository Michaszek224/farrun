# FarRun

A (hopefully great, in the future) project to track and analyze my runs

## What it does

- Reads `.tcx` files exported from Garmin, stored in the `activities` folder :white_check_mark:
- Stores the runs in a SQLite database :white_check_mark:
- Shows them in a web UI :x:

## Status

Early days. Right now it parses `.tcx` files, saves them to SQLite and prints a
summary of each run (date, distance, time). The web UI is next.

## Running it

Put your `.tcx` files in `activities/` and run:

    go run ./cmd/farrun import

Or you can just use anon data:

    go run ./cmd/farrun import --anon

Runs are stored in `farrun.db` by default. You can choose another path with:

    go run ./cmd/farrun import --db /path/to/farrun.db

`--anon` uses a separate `anon.db` by default, so demo data never ends up in
your real database (unless you point `--db` at it yourself).

Times are printed in `Europe/Warsaw`; change it with `--tz America/New_York`.

Importing the same files again is safe: activities are matched by source and
external ID, so they are not duplicated. The whole import is one transaction,
so it either fully succeeds or leaves the database untouched. A `.tcx` file that
cannot be parsed is skipped with a warning and does not stop the others.

The schema is versioned (`PRAGMA user_version`) and upgraded automatically when
the database is opened. Migrations live in `internal/store/migrations`.

Needs Go 1.27 or newer.

## Layout

- `cmd/farrun` - CLI entry point and wiring
- `internal/importer` - parses a folder of `.tcx` files and saves them
- `internal/model` - domain types
- `internal/source` - `.tcx` parser
- `internal/store` - SQLite persistence

## Notes

The `activities` folder is gitignored, because the files contain GPS tracks.
The local databases (`*.db`, along with their `-wal`/`-shm` files) are ignored too.
