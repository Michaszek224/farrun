package source

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaszek224/farrun/internal/model"
)

type TCX struct {
	XMLName    xml.Name   `xml:"TrainingCenterDatabase"`
	Activities []Activity `xml:"Activities>Activity"`
}

type Activity struct {
	Sport  string    `xml:"Sport,attr"`
	ID     time.Time `xml:"Id"`
	Laps   []Lap     `xml:"Lap"`
	Device string    `xml:"Creator>Name"`
}

type Lap struct {
	StartTime        time.Time    `xml:"StartTime,attr"`
	TotalTimeSeconds float64      `xml:"TotalTimeSeconds"`
	DistanceMeters   float64      `xml:"DistanceMeters"`
	Calories         int          `xml:"Calories"`
	AvgHR            int          `xml:"AverageHeartRateBpm>Value"`
	MaxHR            int          `xml:"MaximumHeartRateBpm>Value"`
	Trackpoints      []Trackpoint `xml:"Track>Trackpoint"`
}

type Trackpoint struct {
	Time           time.Time `xml:"Time"`
	Position       *Position `xml:"Position"`
	AltitudeMeters *float64  `xml:"AltitudeMeters"`
	DistanceMeters float64   `xml:"DistanceMeters"`
	HR             *int      `xml:"HeartRateBpm>Value"`
	Speed          float64   `xml:"Extensions>TPX>Speed"`
	Cadence        *int      `xml:"Extensions>TPX>RunCadence"`
}

type Position struct {
	Lat float64 `xml:"LatitudeDegrees"`
	Lon float64 `xml:"LongitudeDegrees"`
}

func parseTCX(path string) (*TCX, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var tcx TCX

	err = xml.NewDecoder(f).Decode(&tcx)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &tcx, nil
}

func ParseAllFiles(dir string) ([]*TCX, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	var tcxList []*TCX
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".tcx") {
			continue
		}

		tcx, err := parseTCX(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}

		tcxList = append(tcxList, tcx)
	}
	return tcxList, nil
}

func (t *TCX) ToActivities() []model.Activity {
	var activities []model.Activity

	for _, a := range t.Activities {
		activity := model.Activity{
			Source:     "Garmin TCX",
			ExternalID: a.ID.UTC().Format(time.RFC3339),
			Sport:      a.Sport,
			StartTime:  a.ID.UTC(),
		}
		for _, l := range a.Laps {
			activity.DistanceMeters += l.DistanceMeters
			activity.DurationSeconds += l.TotalTimeSeconds

			for _, tp := range l.Trackpoints {
				activity.Points = append(activity.Points, tp.toPoint())
			}
		}
		activities = append(activities, activity)
	}

	return activities
}

func (tp Trackpoint) toPoint() model.Point {
	p := model.Point{
		Time:           tp.Time.UTC(),
		AltitudeMeters: tp.AltitudeMeters,
		HR:             tp.HR,
		Cadence:        tp.Cadence,
	}

	if tp.Position != nil {
		p.Position = &model.Position{Lat: tp.Position.Lat, Lon: tp.Position.Lon}
	}

	return p
}
