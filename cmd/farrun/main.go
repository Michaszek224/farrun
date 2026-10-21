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
		for _, a := range tcx.Activities {
			totalDistance := 0.0
			totalSeconds := 0.0
			for _, l := range a.Laps {
				totalDistance += l.DistanceMeters
				totalSeconds += l.TotalTimeSeconds
			}
			secs := int(totalSeconds)
			fmt.Printf("%s %s: %.0fm, %dm:%02ds\n", a.Sport, a.Id.In(loc).Format(time.DateOnly), totalDistance, secs/60, secs%60)
		}
	}
}
