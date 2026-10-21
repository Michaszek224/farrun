package main

import (
	"fmt"
	"log"
	"time"

	"github.com/michaszek224/farrun/internal/source"
)

func main() {
	tcxList, err := source.ParseAllFiles("activities")
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
		}
	}
}
