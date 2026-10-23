// Draws the charts on a page. Every [data-charts] section lists the runs to
// plot as <li data-series-url data-label> and holds one <canvas data-metric>
// per chart. The same code serves the detail page (one run) and the compare
// page (several runs overlaid by distance).
(() => {
  'use strict';

  // The palette lives in style.css (--series-0 ...) so chart lines and the
  // table swatches next to them always match, in light and dark mode.
  const SERIES_COLORS = 6;
  const minSec = (sec) => {
    const t = Math.round(sec);
    return `${Math.floor(t / 60)}:${String(t % 60).padStart(2, '0')}`;
  };

  const METRICS = {
    pace:     { field: 'paceSecPerKm',   unit: 'min/km', format: minSec, reverse: true },
    hr:       { field: 'hr',             unit: 'bpm',    format: (v) => String(Math.round(v)), whole: true },
    altitude: { field: 'altitudeMeters', unit: 'm',      format: (v) => v.toFixed(1) },
    cadence:  { field: 'cadence',        unit: 'spm',    format: (v) => String(Math.round(v)), whole: true },
  };

  const cssVar = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  const seriesColor = (i) => cssVar(`--series-${i % SERIES_COLORS}`);

  // Pace in seconds per km: contain every value, with tick steps that read as
  // clock times (5:00, 5:30, ...) instead of whatever Chart.js happens to pick.
  const PACE_STEPS = [5, 10, 15, 30, 60, 120, 300];
  const paceAxis = (values) => {
    const lo = Math.min(...values);
    const hi = Math.max(...values);
    const step = PACE_STEPS.find((st) => (hi - lo) / st <= 8) || PACE_STEPS[PACE_STEPS.length - 1];
    return {
      min: Math.floor(lo / step) * step - (lo === hi ? step : 0),
      max: Math.ceil(hi / step) * step + (lo === hi ? step : 0),
      ticks: { stepSize: step },
    };
  };

  const fetchRun = async (item) => {
    const res = await fetch(item.dataset.seriesUrl);
    if (!res.ok) throw new Error(`${item.dataset.label}: HTTP ${res.status}`);
    const body = await res.json();
    return { label: item.dataset.label, points: body.points };
  };

  const drawChart = (canvas, runs) => {
    const metric = METRICS[canvas.dataset.metric];
    if (!metric) return false;

    const datasets = runs.map((run, i) => ({
      label: run.label,
      data: run.points.map((p) => ({ x: p.distanceMeters / 1000, y: p[metric.field] })),
      borderColor: seriesColor(i),
      backgroundColor: seriesColor(i),
      borderWidth: 1.5,
      pointRadius: 0,
      pointHoverRadius: 3,
      tension: 0.25,
      spanGaps: false,
    }));

    const values = datasets.flatMap((d) => d.data.map((p) => p.y)).filter((y) => y !== null);
    if (values.length === 0) return false; // nothing recorded for this metric

    // Whole-number metrics need whole-number ticks, or a narrow range such as
    // 75-80 gets half-steps that round to repeated labels (80, 80, 79, 79).
    const y = {
      reverse: !!metric.reverse,
      ticks: { callback: (v) => metric.format(v), ...(metric.whole ? { precision: 0 } : {}) },
      grid: { color: cssVar('--line') },
    };
    if (canvas.dataset.metric === 'pace') {
      const axis = paceAxis(values);
      Object.assign(y, axis, { ticks: { ...y.ticks, ...axis.ticks } });
    }

    // Every chart shares the same x range: from the start to the end of the
    // longest run. Left to itself, Chart.js sizes each axis from the points it
    // happens to plot, so a metric with gaps near the end would stop short.
    const endKm = Math.max(0, ...datasets.map((d) => (d.data.length ? d.data[d.data.length - 1].x : 0)));

    new Chart(canvas, {
      type: 'line',
      data: { datasets },
      options: {
        parsing: false,
        animation: false,
        maintainAspectRatio: false,
        interaction: { mode: 'nearest', axis: 'x', intersect: false },
        scales: {
          x: { type: 'linear', min: 0, max: endKm, title: { display: true, text: 'Distance (km)' }, grid: { color: cssVar('--line') } },
          y,
        },
        plugins: {
          legend: { display: runs.length > 1 },
          tooltip: {
            callbacks: {
              title: (items) => `${items[0].parsed.x.toFixed(2)} km`,
              label: (ctx) => {
                const v = ctx.parsed.y;
                const text = v === null ? '–' : `${metric.format(v)} ${metric.unit}`;
                return runs.length > 1 ? `${ctx.dataset.label}: ${text}` : text;
              },
            },
          },
        },
      },
    });
    return true;
  };

  const init = async (section) => {
    const status = section.querySelector('[data-charts-status]');
    const say = (text) => { if (status) status.textContent = text; };

    if (typeof Chart === 'undefined') {
      say('Charts could not be loaded.');
      return;
    }

    Chart.defaults.color = cssVar('--muted');
    Chart.defaults.font.family = getComputedStyle(document.body).fontFamily;

    try {
      const items = [...section.querySelectorAll('[data-series-url]')];
      const runs = await Promise.all(items.map(fetchRun));

      let drawn = 0;
      for (const canvas of section.querySelectorAll('canvas[data-metric]')) {
        const figure = canvas.closest('figure');
        if (drawChart(canvas, runs)) {
          figure.hidden = false;
          drawn++;
        }
      }
      if (drawn === 0) {
        say('No chart data was recorded for this run.');
      } else if (status) {
        status.hidden = true;
      }
    } catch (err) {
      say(`Charts could not be loaded (${err.message}).`);
    }
  };

  document.querySelectorAll('[data-charts]').forEach(init);
})();
