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
}

type Point struct {
	Time           time.Time
	Lat, Lon       *float64 //in case nil -> no gps
	AltitudeMeters float64
	HR             int
	Cadence        int
}
