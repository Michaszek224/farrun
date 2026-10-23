// Package analysis derives numbers from stored activities: pace, heart rate,
// elevation gain and chart series. It is pure: no database, no HTTP.
package analysis

import (
	"math"
	"sort"

	"github.com/michaszek224/farrun/internal/model"
)

// minMovingSpeed (m/s, about 16:40 min/km) separates moving from standing
// still. Slower samples have no meaningful pace, and plotting them would stretch
// the pace axis to absurd values at every start, stop and traffic light.
// Walking (about 10-12 min/km) is well above it and is still shown.
const minMovingSpeed = 1.0

// rampUpSeconds is how long after standing still a speed reading is ignored.
// Moving off from a standstill is acceleration, not pace: the first sample of a
// run is often 10:30/km on the way to 7:00/km, and showing it would stretch the
// pace axis to fit a number that means nothing.
const rampUpSeconds = 5.0

// Sensors glitch: a watch may report a cadence of 116 for one sample in a run
// that sits at 80, and one such point stretches a chart's axis until the real
// data is a flat line. A sample is a spike, and dropped, when it is further from
// the median of its neighbours than both the noise of those neighbours and a
// minimum that real changes of that metric stay under (spikeFloor*).
const (
	spikeHalfWindow = 5 // neighbours looked at on each side
	spikeMADs       = 3 // how many (scaled) median absolute deviations count as a spike

	spikeFloorCadence = 20.0 // steps per minute
	spikeFloorHR      = 25.0 // beats per minute
	spikeFloorSpeed   = 1.5  // meters per second
)

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
			cadence:  cadence(p.Cadence),
		}
	}

	// Ramp-up first: it needs the raw zeros that mark a standstill, which the
	// spike filter would remove from a short stop as if it were a glitch.
	dropRampUp(samples)
	despike(samples)

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

// dropRampUp clears the speed of samples that follow a standstill too closely.
func dropRampUp(samples []sample) {
	lastStill, stood := 0.0, false
	for i := range samples {
		s := &samples[i]
		if s.speed == nil {
			continue
		}
		if *s.speed < minMovingSpeed {
			lastStill, stood = s.elapsed, true
			continue
		}
		if stood && s.elapsed-lastStill < rampUpSeconds {
			s.speed = nil
		}
	}
}

// despike removes isolated glitches from the metrics that are prone to them.
// It only looks at runs of samples shorter than the window, so a sustained
// change (a walking break, a heart rate ramp) is never touched.
func despike(samples []sample) {
	cadence := make([]*float64, len(samples))
	hr := make([]*float64, len(samples))
	speed := make([]*float64, len(samples))
	for i, s := range samples {
		cadence[i], hr[i], speed[i] = s.cadence, s.hr, s.speed
	}

	cadence = withoutSpikes(cadence, spikeFloorCadence)
	hr = withoutSpikes(hr, spikeFloorHR)
	speed = withoutSpikes(speed, spikeFloorSpeed)

	for i := range samples {
		samples[i].cadence, samples[i].hr, samples[i].speed = cadence[i], hr[i], speed[i]
	}
}

// withoutSpikes returns a copy of values where every spike is nil. A value is a
// spike when it differs from the median of its window by more than both
// spikeMADs scaled MADs and floor. The floor stops a perfectly steady run
// (MAD 0) from having every 1-step wobble flagged.
func withoutSpikes(values []*float64, floor float64) []*float64 {
	out := make([]*float64, len(values))
	copy(out, values)

	window := make([]float64, 0, 2*spikeHalfWindow+1)
	deviations := make([]float64, 0, 2*spikeHalfWindow+1)

	for i, v := range values {
		if v == nil {
			continue
		}

		window = window[:0]
		for j := max(0, i-spikeHalfWindow); j <= min(len(values)-1, i+spikeHalfWindow); j++ {
			if values[j] != nil {
				window = append(window, *values[j])
			}
		}
		if len(window) < 3 {
			continue // too little context to call anything a spike
		}

		med := median(window)
		deviations = deviations[:0]
		for _, w := range window {
			deviations = append(deviations, math.Abs(w-med))
		}
		// 1.4826 scales the MAD to match a standard deviation for normal noise.
		limit := max(spikeMADs*1.4826*median(deviations), floor)

		if math.Abs(*v-med) > limit {
			out[i] = nil
		}
	}
	return out
}

// median sorts its argument.
func median(xs []float64) float64 {
	sort.Float64s(xs)
	mid := len(xs) / 2
	if len(xs)%2 == 1 {
		return xs[mid]
	}
	return (xs[mid-1] + xs[mid]) / 2
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

// cadence treats 0 as "no data": watches report 0 while you stand still, and
// those samples would drag the chart's axis down to zero and flatten the line.
func cadence(v *int) *float64 {
	if v == nil || *v <= 0 {
		return nil
	}
	return toFloat(v)
}

func toFloat(v *int) *float64 {
	if v == nil {
		return nil
	}
	f := float64(*v)
	return &f
}
