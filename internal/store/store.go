package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/michaszek224/farrun/internal/model"
)

var ErrNotFound = errors.New("activity not found")

type Store struct {
	db *sql.DB
}

// dsn builds the driver-specific connection string. It is the only place that
// knows about the sqlite driver's syntax.
func dsn(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: q.Encode()}
	return u.String(), nil
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("open sqlite: empty path")
	}

	conn, err := dsn(path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	db, err := sql.Open("sqlite", conn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}

	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to sqlite %q: %w", path, err)
	}

	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate %q: %w", path, err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// validate rejects data the database could not represent faithfully, before
// anything is written.
func validate(a model.Activity) error {
	switch {
	case a.Source == "":
		return errors.New("empty source")
	case a.ExternalID == "":
		return errors.New("empty external id")
	case a.StartTime.IsZero():
		return errors.New("missing start time")
	}
	for i, p := range a.Points {
		if p.Time.IsZero() {
			return fmt.Errorf("trackpoint %d has no time", i)
		}
	}
	return nil
}

// SaveActivities imports activities in a single transaction: either all new
// activities are stored, or none are. Activities that are already stored
// (same source and external id) are skipped. It returns how many were new.
func (s *Store) SaveActivities(ctx context.Context, activities []model.Activity) (int, error) {
	for _, a := range activities {
		if err := validate(a); err != nil {
			return 0, fmt.Errorf("invalid activity %q: %w", a.ExternalID, err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	activityStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO activities
			(source, external_id, sport, device, start_time_ms, distance_meters, duration_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (source, external_id) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("prepare activity insert: %w", err)
	}
	defer activityStmt.Close()

	lapStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO laps
			(activity_id, idx, start_time_ms, duration_seconds, distance_meters, calories, avg_heart_rate, max_heart_rate)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare lap insert: %w", err)
	}
	defer lapStmt.Close()

	pointStmt, err := tx.PrepareContext(ctx, `
		INSERT INTO trackpoints
			(activity_id, time_ms, latitude, longitude, altitude_meters, distance_meters, speed_mps, heart_rate, cadence)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return 0, fmt.Errorf("prepare trackpoint insert: %w", err)
	}
	defer pointStmt.Close()

	inserted := 0
	for _, a := range activities {
		res, err := activityStmt.ExecContext(ctx,
			a.Source,
			a.ExternalID,
			a.Sport,
			a.Device,
			a.StartTime.UTC().UnixMilli(),
			a.DistanceMeters,
			a.DurationSeconds,
		)
		if err != nil {
			return 0, fmt.Errorf("insert activity %q: %w", a.ExternalID, err)
		}

		affected, err := res.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("check activity %q: %w", a.ExternalID, err)
		}
		if affected == 0 {
			continue // already imported
		}

		activityID, err := res.LastInsertId()
		if err != nil {
			return 0, fmt.Errorf("get id of activity %q: %w", a.ExternalID, err)
		}

		for i, l := range a.Laps {
			var start any
			if !l.StartTime.IsZero() {
				start = l.StartTime.UTC().UnixMilli()
			}

			_, err := lapStmt.ExecContext(ctx,
				activityID,
				i,
				start,
				l.DurationSeconds,
				l.DistanceMeters,
				nullable(l.Calories),
				nullable(l.AvgHR),
				nullable(l.MaxHR),
			)
			if err != nil {
				return 0, fmt.Errorf("insert lap %d of activity %q: %w", i, a.ExternalID, err)
			}
		}

		for _, p := range a.Points {
			var lat, lon any
			if p.Position != nil {
				lat, lon = p.Position.Lat, p.Position.Lon
			}

			_, err := pointStmt.ExecContext(ctx,
				activityID,
				p.Time.UTC().UnixMilli(),
				lat,
				lon,
				nullable(p.AltitudeMeters),
				nullable(p.DistanceMeters),
				nullable(p.SpeedMPS),
				nullable(p.HR),
				nullable(p.Cadence),
			)
			if err != nil {
				return 0, fmt.Errorf("insert trackpoint for activity %q: %w", a.ExternalID, err)
			}
		}

		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit import: %w", err)
	}

	return inserted, nil
}

const activityColumns = `id, source, external_id, sport, device, start_time_ms, distance_meters, duration_seconds`

type scanner interface {
	Scan(dest ...any) error
}

func scanActivity(row scanner) (model.Activity, error) {
	var (
		a         model.Activity
		startTime int64
	)
	err := row.Scan(
		&a.ID,
		&a.Source,
		&a.ExternalID,
		&a.Sport,
		&a.Device,
		&startTime,
		&a.DistanceMeters,
		&a.DurationSeconds,
	)
	if err != nil {
		return model.Activity{}, err
	}
	a.StartTime = time.UnixMilli(startTime).UTC()
	return a, nil
}

// ListActivities returns all activities, newest first, without laps and
// trackpoints. Use Activity to load those.
func (s *Store) ListActivities(ctx context.Context) ([]model.Activity, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+activityColumns+`
		FROM activities
		ORDER BY start_time_ms DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list activities: %w", err)
	}
	defer rows.Close()

	var activities []model.Activity
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, fmt.Errorf("scan activity: %w", err)
		}
		activities = append(activities, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate activities: %w", err)
	}

	return activities, nil
}

// Activity returns one activity with all of its laps and trackpoints.
func (s *Store) Activity(ctx context.Context, id int64) (model.Activity, error) {
	a, err := scanActivity(s.db.QueryRowContext(ctx, `
		SELECT `+activityColumns+`
		FROM activities
		WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Activity{}, ErrNotFound
	}
	if err != nil {
		return model.Activity{}, fmt.Errorf("get activity %d: %w", id, err)
	}

	if a.Laps, err = s.laps(ctx, id); err != nil {
		return model.Activity{}, err
	}
	if a.Points, err = s.points(ctx, id); err != nil {
		return model.Activity{}, err
	}

	return a, nil
}

func (s *Store) laps(ctx context.Context, activityID int64) ([]model.Lap, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT start_time_ms, duration_seconds, distance_meters, calories, avg_heart_rate, max_heart_rate
		FROM laps
		WHERE activity_id = ?
		ORDER BY idx`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get laps of activity %d: %w", activityID, err)
	}
	defer rows.Close()

	var laps []model.Lap
	for rows.Next() {
		var (
			l                      model.Lap
			start                  sql.Null[int64]
			calories, avgHR, maxHR sql.Null[int]
		)
		if err := rows.Scan(&start, &l.DurationSeconds, &l.DistanceMeters, &calories, &avgHR, &maxHR); err != nil {
			return nil, fmt.Errorf("scan lap of activity %d: %w", activityID, err)
		}

		if start.Valid {
			l.StartTime = time.UnixMilli(start.V).UTC()
		}
		l.Calories = pointer(calories)
		l.AvgHR = pointer(avgHR)
		l.MaxHR = pointer(maxHR)

		laps = append(laps, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate laps of activity %d: %w", activityID, err)
	}

	return laps, nil
}

func (s *Store) points(ctx context.Context, activityID int64) ([]model.Point, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT time_ms, latitude, longitude, altitude_meters, distance_meters, speed_mps, heart_rate, cadence
		FROM trackpoints
		WHERE activity_id = ?
		ORDER BY time_ms, id`, activityID)
	if err != nil {
		return nil, fmt.Errorf("get trackpoints of activity %d: %w", activityID, err)
	}
	defer rows.Close()

	var points []model.Point
	for rows.Next() {
		var (
			p                model.Point
			timeMS           int64
			lat, lon         sql.Null[float64]
			alt, dist, speed sql.Null[float64]
			hr, cadence      sql.Null[int]
		)
		if err := rows.Scan(&timeMS, &lat, &lon, &alt, &dist, &speed, &hr, &cadence); err != nil {
			return nil, fmt.Errorf("scan trackpoint of activity %d: %w", activityID, err)
		}

		p.Time = time.UnixMilli(timeMS).UTC()
		if lat.Valid && lon.Valid {
			p.Position = &model.Position{Lat: lat.V, Lon: lon.V}
		}
		p.AltitudeMeters = pointer(alt)
		p.DistanceMeters = pointer(dist)
		p.SpeedMPS = pointer(speed)
		p.HR = pointer(hr)
		p.Cadence = pointer(cadence)

		points = append(points, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trackpoints of activity %d: %w", activityID, err)
	}

	return points, nil
}

func (s *Store) CountActivities(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM activities`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count activities: %w", err)
	}
	return count, nil
}

// nullable turns an optional value into a query argument: nil becomes SQL NULL.
func nullable[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// pointer is the reverse of nullable: SQL NULL becomes nil.
func pointer[T any](n sql.Null[T]) *T {
	if !n.Valid {
		return nil
	}
	v := n.V
	return &v
}
