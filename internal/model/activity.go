package model

import "time"

type Activity struct {
	ID              int64
	Source          string //for now tcx, aymbe strava connection in future
	ExternalID      string
	Sport           string
	StartTime       time.Time //UTC to convert later
	DistanceMeters  float64
	DurationSeconds float64
	Points          []Point
}

type Point struct {
	Time           time.Time
	Position       *Position //nil -> no data
	AltitudeMeters *float64  //nil -> no data
	HR             *int      //nil -> no data
	Cadence        *int      //nil -> no data
}

type Position struct {
	Lat float64
	Lon float64
}
