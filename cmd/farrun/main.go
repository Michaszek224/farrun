package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"
	_ "time/tzdata" // so --tz works on machines without a system zoneinfo

	"github.com/michaszek224/farrun/internal/model"
	"github.com/michaszek224/farrun/internal/source"
	"github.com/michaszek224/farrun/internal/store"
)

const (
	defaultDB     = "farrun.db"
	defaultAnonDB = "anon.db"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		log.Fatal(err)
	}
}

func run(args []string, out, errOut io.Writer) (err error) {
	fs := flag.NewFlagSet("farrun", flag.ContinueOnError)
	fs.SetOutput(errOut)
	anon := fs.Bool("anon", false, "read anonymized files from anonActivities")
	dbPath := fs.String("db", "", "path to the SQLite database (default "+defaultDB+", or "+defaultAnonDB+" with --anon)")
	tz := fs.String("tz", "Europe/Warsaw", "time zone used to print times")
	if err := fs.Parse(args); err != nil {
		return err
	}

	dir := "activities"
	if *anon {
		dir = "anonActivities"
	}

	// Demo data must never end up in the real database.
	if *dbPath == "" {
		*dbPath = defaultDB
		if *anon {
			*dbPath = defaultAnonDB
		}
	}

	loc, err := time.LoadLocation(*tz)
	if err != nil {
		return fmt.Errorf("load time zone %q: %w", *tz, err)
	}

	tcxList, skipped, err := source.ParseAllFiles(dir)
	if err != nil {
		return err
	}
	for _, e := range skipped {
		fmt.Fprintf(errOut, "warning: skipped file: %v\n", e)
	}

	var activities []model.Activity
	for _, tcx := range tcxList {
		activities = append(activities, tcx.ToActivities()...)
	}

	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close database: %w", closeErr)
		}
	}()

	ctx := context.Background()

	inserted, err := db.SaveActivities(ctx, activities)
	if err != nil {
		return err
	}

	total, err := db.CountActivities(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "Imported %d new activities (%d in database", inserted, total)
	if len(skipped) > 0 {
		fmt.Fprintf(out, ", %d files skipped", len(skipped))
	}
	fmt.Fprint(out, ")\n\n")
	for _, a := range activities {
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
