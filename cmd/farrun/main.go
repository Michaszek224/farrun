package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/michaszek224/farrun/internal/source"
)

func main() {
	anon := flag.Bool("anon", false, "read annoymized files from annonActivities")
	flag.Parse()

	dir := "activities"
	if *anon {
		dir = "anonActivities"
	}

	tcxList, err := source.ParseAllFiles(dir)
	if err != nil {
		log.Fatal(err)
	}

	loc, err := time.LoadLocation("Europe/Warsaw")
	if err != nil {
		log.Fatal(err)
	}

	for _, tcx := range tcxList {
		for _, a := range tcx.ToActivities() {
			secs := int(a.DurationSeconds)
			fmt.Printf("%s %s: %.2fkm, %d:%02dm\n",
				a.Sport,
				a.StartTime.In(loc).Format(time.DateOnly),
				a.DistanceMeters/1000,
				secs/60, secs%60)

			fmt.Printf("  number of points: %d\n", len(a.Points))
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

				fmt.Printf("  #%d %s | gps: %s | wys: %s | hr: %s | kad: %s\n",
					i, p.Time.In(loc).Format(time.TimeOnly), gps, alt, hr, cad)
			}
			fmt.Println()
		}
	}
}
