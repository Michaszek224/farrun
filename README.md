# FarRun

A (hopefully great, in the future) project to track and analyze my runs

## What it does ()

- Reads `.tcx` files exported from Garmin, stored in the `activities` folder :white_check_mark:
- Stores the runs in a database :x:
- Shows them in a web UI :x:

## Status

Early days. Right now it only parses `.tcx` files and prints a summary of each run
(date, distance, time). Database and web UI are next.

## Running it

Put your `.tcx` files in `activities/` and run:

    go run ./cmd/farrun

Or you can just use anon data:

    go run ./cmd/farrun --anon

Needs Go 1.20 or newer.

## Notes

The `activities` folder is gitignored, because the files contain GPS tracks.
