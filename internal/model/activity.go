package model

import "time"

type Activity struct {
	ID              int64
	Source          string //for now tcx, aymbe strava connection in future
	ExternalID      string
	Sport           string
	Device          string    //"" -> unknown
	StartTime       time.Time //UTC to convert later
	DistanceMeters  float64
	DurationSeconds float64
	Laps            []Lap
	Points          []Point
}

type Lap struct {
	StartTime       time.Time
	DurationSeconds float64
	DistanceMeters  float64
	Calories        *int //nil -> no data
	AvgHR           *int //nil -> no data
	MaxHR           *int //nil -> no data
}

type Point struct {
	Time           time.Time
	Position       *Position //nil -> no data
	AltitudeMeters *float64  //nil -> no data
	DistanceMeters *float64  //cumulative distance in meters
	SpeedMPS       *float64  //meters per second
	HR             *int      //nil -> no data
	Cadence        *int      //nil -> no data
}

type Position struct {
	Lat float64
	Lon float64
}
