// Package analysis derives numbers from stored activities: pace, heart rate,
// elevation gain and chart series. It is pure: no database, no HTTP.
package analysis

import (
	"github.com/michaszek224/farrun/internal/model"
)

// minMovingSpeed (m/s, about 33 min/km) separates running from standing still.
// Slower samples have no meaningful pace.
const minMovingSpeed = 0.5

// elevationThreshold is how far the altitude must move before it counts as a
// climb or descent. Barometric altitude jitters by a few tenths of a meter, and
// summing that noise would inflate the gain.
const elevationThreshold = 1.0

// Pace returns seconds per kilometer. ok is false when distance or duration is
// not positive, because there is no pace to speak of.
func Pace(distanceMeters, durationSeconds float64) (secPerKm float64, ok bool) {
	if distanceMeters <= 0 || durationSeconds <= 0 {
		return 0, false
	}
	return durationSeconds / (distanceMeters / 1000), true
}

// AvgHR is the mean of the recorded heart rates. ok is false if no point has one.
func AvgHR(points []model.Point) (bpm float64, ok bool) {
	sum, n := 0, 0
	for _, p := range points {
		if p.HR != nil {
			sum += *p.HR
			n++
		}
	}
	if n == 0 {
		return 0, false
	}
	return float64(sum) / float64(n), true
}

// MaxHR is the highest recorded heart rate. ok is false if no point has one.
func MaxHR(points []model.Point) (bpm int, ok bool) {
	for _, p := range points {
		if p.HR != nil && (!ok || *p.HR > bpm) {
			bpm, ok = *p.HR, true
		}
	}
	return bpm, ok
}

// ElevationGain is the total climb in meters, ignoring altitude changes
// smaller than elevationThreshold. Points without altitude are skipped.
func ElevationGain(points []model.Point) float64 {
	var (
		gain    float64
		ref     float64
		started bool
	)
	for _, p := range points {
		if p.AltitudeMeters == nil {
			continue
		}
		alt := *p.AltitudeMeters

		switch {
		case !started:
			ref, started = alt, true
		case alt >= ref+elevationThreshold:
			gain += alt - ref
			ref = alt
		case alt <= ref-elevationThreshold:
			ref = alt
		}
	}
	return gain
}

// SeriesPoint is one sample on a chart. A nil field means no data.
type SeriesPoint struct {
	DistanceMeters float64  `json:"distanceMeters"`
	ElapsedSeconds float64  `json:"elapsedSeconds"`
	PaceSecPerKm   *float64 `json:"paceSecPerKm"`
	HR             *float64 `json:"hr"`
	AltitudeMeters *float64 `json:"altitudeMeters"`
	Cadence        *float64 `json:"cadence"`
}

// Series converts trackpoints into at most maxPoints chart samples. When there
// are more points than that, neighbouring points are averaged into buckets,
// which also smooths the noisy instantaneous speed. Bucket positions are the
// average of their points, so the first and last samples are close to, but not
// exactly, the first and last trackpoints.
func Series(points []model.Point, maxPoints int) []SeriesPoint {
	if len(points) == 0 {
		return nil
	}
	if maxPoints < 1 {
		maxPoints = 1
	}

	samples := make([]sample, len(points))
	var distance float64
	for i, p := range points {
		if p.DistanceMeters != nil {
			distance = *p.DistanceMeters // otherwise carry the last known distance
		}
		samples[i] = sample{
			distance: distance,
			elapsed:  p.Time.Sub(points[0].Time).Seconds(),
			speed:    p.SpeedMPS,
			hr:       toFloat(p.HR),
			altitude: p.AltitudeMeters,
			cadence:  toFloat(p.Cadence),
		}
	}

	size := (len(samples) + maxPoints - 1) / maxPoints // bucket size, rounded up
	out := make([]SeriesPoint, 0, (len(samples)+size-1)/size)
	for start := 0; start < len(samples); start += size {
		end := min(start+size, len(samples))
		out = append(out, average(samples[start:end]))
	}
	return out
}

type sample struct {
	distance, elapsed float64
	speed             *float64
	hr, altitude      *float64
	cadence           *float64
}

func average(bucket []sample) SeriesPoint {
	var distance, elapsed float64
	speed, hr, altitude, cadence := &mean{}, &mean{}, &mean{}, &mean{}
	for _, s := range bucket {
		distance += s.distance
		elapsed += s.elapsed
		speed.add(s.speed)
		hr.add(s.hr)
		altitude.add(s.altitude)
		cadence.add(s.cadence)
	}
	n := float64(len(bucket))

	sp := SeriesPoint{
		DistanceMeters: distance / n,
		ElapsedSeconds: elapsed / n,
		HR:             hr.value(),
		AltitudeMeters: altitude.value(),
		Cadence:        cadence.value(),
	}
	// Average the speed, then convert: averaging paces would overweight slow samples.
	if v := speed.value(); v != nil && *v >= minMovingSpeed {
		pace := 1000 / *v
		sp.PaceSecPerKm = &pace
	}
	return sp
}

// mean averages the values it is given, ignoring nil.
type mean struct {
	sum float64
	n   int
}

func (m *mean) add(v *float64) {
	if v != nil {
		m.sum += *v
		m.n++
	}
}

func (m *mean) value() *float64 {
	if m.n == 0 {
		return nil
	}
	v := m.sum / float64(m.n)
	return &v
}

func toFloat(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}
