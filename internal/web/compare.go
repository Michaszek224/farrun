package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/michaszek224/farrun/internal/store"
)

const (
	minCompare = 2
	maxCompare = 10
)

type compareRun struct {
	detailPage
	Index int // position in the selection; picks the color
}

type comparePage struct {
	Runs []compareRun
}

// handleCompare shows several activities side by side. The selection comes
// from repeated id parameters: /compare?id=3&id=7.
func (s *server) handleCompare(w http.ResponseWriter, r *http.Request) {
	ids, msg := parseCompareIDs(r.URL.Query()["id"])
	if msg != "" {
		s.errorPage(w, r, http.StatusBadRequest, msg)
		return
	}

	page := comparePage{Runs: make([]compareRun, 0, len(ids))}
	for i, id := range ids {
		a, err := s.store.Activity(r.Context(), id)
		if errors.Is(err, store.ErrNotFound) {
			s.errorPage(w, r, http.StatusNotFound, "One of the selected activities no longer exists.")
			return
		}
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		page.Runs = append(page.Runs, compareRun{detailPage: s.newDetailPage(a), Index: i})
	}

	s.render(w, r, "compare", http.StatusOK, page)
}

// parseCompareIDs turns the raw id parameters into a list without duplicates,
// in the order given. When the selection is unusable it returns a message for
// the user instead.
func parseCompareIDs(raw []string) (ids []int64, problem string) {
	seen := map[int64]bool{}
	for _, v := range raw {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id < 1 {
			return nil, "One of the selected activity ids is not valid."
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}

	switch {
	case len(ids) < minCompare:
		return nil, "Select at least two activities to compare."
	case len(ids) > maxCompare:
		return nil, "You can compare at most " + strconv.Itoa(maxCompare) + " activities at once."
	}
	return ids, ""
}
