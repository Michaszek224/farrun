package importer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaszek224/farrun/internal/store"
)

func tcx(id string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<TrainingCenterDatabase>
  <Activities>
    <Activity Sport="Running">
      <Id>` + id + `</Id>
      <Lap StartTime="` + id + `">
        <TotalTimeSeconds>60</TotalTimeSeconds>
        <DistanceMeters>200</DistanceMeters>
        <Track>
          <Trackpoint>
            <Time>` + id + `</Time>
            <DistanceMeters>0</DistanceMeters>
          </Trackpoint>
        </Track>
      </Lap>
    </Activity>
  </Activities>
</TrainingCenterDatabase>`
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.tcx", tcx("2026-10-20T06:30:00Z"))
	writeFile(t, dir, "b.tcx", tcx("2026-10-21T06:30:00Z"))
	st := openStore(t)
	ctx := context.Background()

	res, err := Run(ctx, dir, st)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Inserted != 2 || res.Total != 2 || len(res.Activities) != 2 || len(res.Skipped) != 0 {
		t.Fatalf("Run() = inserted %d, total %d, activities %d, skipped %d; want 2, 2, 2, 0",
			res.Inserted, res.Total, len(res.Activities), len(res.Skipped))
	}

	// Importing again finds the same activities but stores none of them.
	res, err = Run(ctx, dir, st)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if res.Inserted != 0 || res.Total != 2 || len(res.Activities) != 2 {
		t.Fatalf("second Run() = inserted %d, total %d, activities %d; want 0, 2, 2",
			res.Inserted, res.Total, len(res.Activities))
	}
}

func TestRunSkipsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.tcx", tcx("2026-10-20T06:30:00Z"))
	writeFile(t, dir, "bad.tcx", "not xml")

	res, err := Run(context.Background(), dir, openStore(t))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Inserted != 1 {
		t.Errorf("Inserted = %d, want 1", res.Inserted)
	}
	if len(res.Skipped) != 1 || !strings.Contains(res.Skipped[0].Error(), "bad.tcx") {
		t.Errorf("Skipped = %v, want only bad.tcx", res.Skipped)
	}
}

func TestRunEmptyDirectory(t *testing.T) {
	res, err := Run(context.Background(), t.TempDir(), openStore(t))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if res.Inserted != 0 || res.Total != 0 {
		t.Errorf("Run() = inserted %d, total %d; want 0, 0", res.Inserted, res.Total)
	}
}

func TestRunMissingDirectory(t *testing.T) {
	_, err := Run(context.Background(), filepath.Join(t.TempDir(), "missing"), openStore(t))
	if err == nil {
		t.Fatal("Run() error = nil, want an error")
	}
}
