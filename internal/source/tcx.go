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
	Calories         *int         `xml:"Calories"`
	AvgHR            *int         `xml:"AverageHeartRateBpm>Value"`
	MaxHR            *int         `xml:"MaximumHeartRateBpm>Value"`
	Trackpoints      []Trackpoint `xml:"Track>Trackpoint"`
}

type Trackpoint struct {
	Time           time.Time `xml:"Time"`
	Position       *Position `xml:"Position"`
	AltitudeMeters *float64  `xml:"AltitudeMeters"`
	DistanceMeters *float64  `xml:"DistanceMeters"`
	HR             *int      `xml:"HeartRateBpm>Value"`
	Speed          *float64  `xml:"Extensions>TPX>Speed"`
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

// ParseAllFiles parses every .tcx file directly inside dir. A file that cannot
// be parsed does not stop the others: it is reported in skipped and the rest
// are still returned. err is only set when dir itself cannot be read.
func ParseAllFiles(dir string) (tcxList []*TCX, skipped []error, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".tcx") {
			continue
		}

		tcx, err := parseTCX(filepath.Join(dir, e.Name()))
		if err != nil {
			skipped = append(skipped, err)
			continue
		}

		tcxList = append(tcxList, tcx)
	}
	return tcxList, skipped, nil
}

// ToActivities converts the file to model activities. Activities without a
// start time cannot be identified, and trackpoints without a time cannot be
// placed on a timeline, so both are dropped.
func (t *TCX) ToActivities() []model.Activity {
	var activities []model.Activity

	for _, a := range t.Activities {
		if a.ID.IsZero() {
			continue
		}

		activity := model.Activity{
			Source:     "Garmin TCX",
			ExternalID: a.ID.UTC().Format(time.RFC3339),
			Sport:      a.Sport,
			Device:     strings.TrimSpace(a.Device),
			StartTime:  a.ID.UTC(),
		}
		for _, l := range a.Laps {
			activity.DistanceMeters += l.DistanceMeters
			activity.DurationSeconds += l.TotalTimeSeconds
			activity.Laps = append(activity.Laps, l.toLap())

			for _, tp := range l.Trackpoints {
				if tp.Time.IsZero() {
					continue
				}
				activity.Points = append(activity.Points, tp.toPoint())
			}
		}
		activities = append(activities, activity)
	}

	return activities
}

func (l Lap) toLap() model.Lap {
	return model.Lap{
		StartTime:       l.StartTime.UTC(),
		DurationSeconds: l.TotalTimeSeconds,
		DistanceMeters:  l.DistanceMeters,
		Calories:        l.Calories,
		AvgHR:           l.AvgHR,
		MaxHR:           l.MaxHR,
	}
}

func (tp Trackpoint) toPoint() model.Point {
	p := model.Point{
		Time:           tp.Time.UTC(),
		AltitudeMeters: tp.AltitudeMeters,
		DistanceMeters: tp.DistanceMeters,
		SpeedMPS:       tp.Speed,
		HR:             tp.HR,
		Cadence:        tp.Cadence,
	}

	if tp.Position != nil {
		p.Position = &model.Position{Lat: tp.Position.Lat, Lon: tp.Position.Lon}
	}

	return p
}
