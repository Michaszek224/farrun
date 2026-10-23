// Package importer turns a folder of .tcx files into stored activities.
package importer

import (
	"context"

	"github.com/michaszek224/farrun/internal/model"
	"github.com/michaszek224/farrun/internal/source"
	"github.com/michaszek224/farrun/internal/store"
)

type Result struct {
	Activities []model.Activity // everything found in the files, new or already stored
	Inserted   int              // how many of them were new
	Total      int              // activities in the database afterwards
	Skipped    []error          // files that could not be parsed
}

// Run parses every .tcx file in dir and saves the activities to st. Bad files
// are reported in Result.Skipped without stopping the import; the save itself
// is all-or-nothing (see store.SaveActivities).
func Run(ctx context.Context, dir string, st *store.Store) (Result, error) {
	tcxList, skipped, err := source.ParseAllFiles(dir)
	if err != nil {
		return Result{}, err
	}

	var activities []model.Activity
	for _, tcx := range tcxList {
		activities = append(activities, tcx.ToActivities()...)
	}

	inserted, err := st.SaveActivities(ctx, activities)
	if err != nil {
		return Result{}, err
	}

	total, err := st.CountActivities(ctx)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Activities: activities,
		Inserted:   inserted,
		Total:      total,
		Skipped:    skipped,
	}, nil
}
