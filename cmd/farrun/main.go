package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"
	_ "time/tzdata" // so --tz works on machines without a system zoneinfo

	"github.com/michaszek224/farrun/internal/importer"
	"github.com/michaszek224/farrun/internal/model"
	"github.com/michaszek224/farrun/internal/store"
)

const (
	defaultDB     = "farrun.db"
	defaultAnonDB = "anon.db"
)

const usage = `usage: farrun <command> [flags]

commands:
  import   read .tcx files into the database and print a summary
`

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(errOut, usage)
		return errors.New("missing command")
	}

	switch args[0] {
	case "import":
		return runImport(args[1:], out, errOut)
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	default:
		fmt.Fprint(errOut, usage)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// dbPath picks the database file. Demo data must never end up in the real
// database, so --anon gets its own default.
func dbPath(flagValue string, anon bool) string {
	switch {
	case flagValue != "":
		return flagValue
	case anon:
		return defaultAnonDB
	default:
		return defaultDB
	}
}

func runImport(args []string, out, errOut io.Writer) (err error) {
	fs := flag.NewFlagSet("farrun import", flag.ContinueOnError)
	fs.SetOutput(errOut)
	anon := fs.Bool("anon", false, "read anonymized files from anonActivities")
	db := fs.String("db", "", "path to the SQLite database (default "+defaultDB+", or "+defaultAnonDB+" with --anon)")
	tz := fs.String("tz", "Europe/Warsaw", "time zone used to print times")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := "activities"
	if *anon {
		dir = "anonActivities"
	}

	loc, err := time.LoadLocation(*tz)
	if err != nil {
		return fmt.Errorf("load time zone %q: %w", *tz, err)
	}

	// Check before opening the database, which would create an empty file.
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("read dir %s: %w", dir, err)
	}

	st, err := store.Open(dbPath(*db, *anon))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	res, err := importer.Run(context.Background(), dir, st)
	if err != nil {
		return err
	}

	for _, e := range res.Skipped {
		fmt.Fprintf(errOut, "warning: skipped file: %v\n", e)
	}

	fmt.Fprintf(out, "Imported %d new activities (%d in database", res.Inserted, res.Total)
	if len(res.Skipped) > 0 {
		fmt.Fprintf(out, ", %d files skipped", len(res.Skipped))
	}
	fmt.Fprint(out, ")\n\n")
	for _, a := range res.Activities {
		printActivity(out, a, loc)
	}

	return nil
}

func printActivity(out io.Writer, a model.Activity, loc *time.Location) {
	secs := int(a.DurationSeconds)
	fmt.Fprintf(out, "%s %s: %.2fkm, %d:%02dm\n",
		a.Sport,
		a.StartTime.In(loc).Format(time.DateOnly),
		a.DistanceMeters/1000,
		secs/60, secs%60)

	fmt.Fprintf(out, "  number of points: %d\n", len(a.Points))
	for i, p := range a.Points {
		if i == 5 {
			break
		}

		gps := "No gps"
		if p.Position != nil {
			gps = fmt.Sprintf("%.5f, %.5f", p.Position.Lat, p.Position.Lon)
		}

		alt, hr, cad := "-", "-", "-"
		if p.AltitudeMeters != nil {
			alt = fmt.Sprintf("%.1f", *p.AltitudeMeters)
		}
		if p.HR != nil {
			hr = fmt.Sprint(*p.HR)
		}
		if p.Cadence != nil {
			cad = fmt.Sprint(*p.Cadence)
		}

		fmt.Fprintf(out, "  #%d %s | gps: %s | wys: %s | hr: %s | kad: %s\n",
			i, p.Time.In(loc).Format(time.TimeOnly), gps, alt, hr, cad)
	}
	fmt.Fprintln(out)
}
