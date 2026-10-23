// Click a column header of the activities table to sort by it. Each header
// carries data-sort-key, and each cell the raw value to sort on (data-sort),
// because the displayed text ("1:02:05", "6:12") does not sort correctly.
//
// A column starts in its "best first" direction: newest, longest, fastest, A-Z.
// Clicking the active column flips it. Clicking another column starts that one
// over at its first direction. The headers become buttons here, not in the
// HTML, so without JavaScript nothing is left that looks clickable but is not.
(() => {
  'use strict';

  const table = document.querySelector('[data-compare-form] table');
  if (!table) return;

  // Where each column starts, and how its values compare.
  const COLUMNS = {
    date:     { first: 'desc', numeric: true },  // newest first
    sport:    { first: 'asc',  numeric: false }, // A-Z
    distance: { first: 'desc', numeric: true },  // longest first
    time:     { first: 'desc', numeric: true },  // longest first
    pace:     { first: 'asc',  numeric: true },  // fastest (fewest seconds per km) first
  };

  const tbody = table.tBodies[0];
  const headers = [...table.querySelectorAll('th[data-sort-key]')];
  const columnIndex = (th) => th.cellIndex;
  const rows = [...tbody.rows];

  // The server lists runs newest first, so that is where we start. A query reloads
  // the page, so the sort is remembered for this tab and picked up again here.
  const STORAGE_KEY = 'farrun.sort';
  const state = { key: 'date', dir: 'desc' };

  try {
    const saved = JSON.parse(sessionStorage.getItem(STORAGE_KEY));
    if (saved && COLUMNS[saved.key] && (saved.dir === 'asc' || saved.dir === 'desc')) {
      state.key = saved.key;
      state.dir = saved.dir;
    }
  } catch { /* storage blocked or corrupt: start from the default */ }

  const cellValue = (row, index) => row.cells[index].dataset.sort;

  // Rows without a value (a pace needs a distance) go last, in either direction.
  const compareBy = (key, dir, index) => {
    const { numeric } = COLUMNS[key];
    const sign = dir === 'asc' ? 1 : -1;

    return (a, b) => {
      const va = cellValue(a, index);
      const vb = cellValue(b, index);
      if (va === '' && vb === '') return 0;
      if (va === '') return 1;
      if (vb === '') return -1;

      const order = numeric
        ? Number(va) - Number(vb)
        : va.localeCompare(vb, undefined, { sensitivity: 'base' });
      return order * sign;
    };
  };

  const dateIndex = columnIndex(headers.find((th) => th.dataset.sortKey === 'date'));

  const apply = () => {
    const th = headers.find((h) => h.dataset.sortKey === state.key);
    const byColumn = compareBy(state.key, state.dir, columnIndex(th));
    const newestFirst = compareBy('date', 'desc', dateIndex);

    // Ties fall back to newest first, so equal values never shuffle.
    rows.sort((a, b) => byColumn(a, b) || newestFirst(a, b));
    tbody.append(...rows); // moves the existing rows, so ticked checkboxes stay ticked

    for (const h of headers) {
      const active = h === th;
      h.setAttribute('aria-sort', active ? (state.dir === 'asc' ? 'ascending' : 'descending') : 'none');
      h.dataset.dir = active ? state.dir : '';
    }
  };

  const choose = (key) => {
    if (state.key === key) {
      state.dir = state.dir === 'asc' ? 'desc' : 'asc';
    } else {
      state.key = key;
      state.dir = COLUMNS[key].first;
    }
    apply();
    try { sessionStorage.setItem(STORAGE_KEY, JSON.stringify(state)); } catch { /* not worth failing for */ }
  };

  for (const th of headers) {
    const key = th.dataset.sortKey;
    if (!COLUMNS[key]) continue;

    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'sort';
    button.textContent = th.textContent;
    th.textContent = '';
    th.append(button);

    button.addEventListener('click', () => choose(key));
  }

  apply();
})();
