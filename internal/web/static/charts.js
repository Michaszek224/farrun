// Draws the charts on a page. Every [data-charts] section lists the runs to
// plot as <li data-series-url data-label> and holds one <canvas data-metric>
// per chart. The same code serves the detail page (one run) and the compare
// page (several runs overlaid by distance).
(() => {
  'use strict';

  // Distinct on light and dark backgrounds, and for common color blindness.
  const COLORS = ['#0b6e4f', '#d1495b', '#2e86ab', '#e08e0b', '#7d5ba6', '#4f6d7a'];

  const minSec = (sec) => {
    const t = Math.round(sec);
    return `${Math.floor(t / 60)}:${String(t % 60).padStart(2, '0')}`;
  };

  const METRICS = {
    pace:     { field: 'paceSecPerKm',   unit: 'min/km', format: minSec, reverse: true },
    hr:       { field: 'hr',             unit: 'bpm',    format: (v) => String(Math.round(v)) },
    altitude: { field: 'altitudeMeters', unit: 'm',      format: (v) => v.toFixed(1) },
    cadence:  { field: 'cadence',        unit: 'spm',    format: (v) => String(Math.round(v)) },
  };

  const cssVar = (name) => getComputedStyle(document.documentElement).getPropertyValue(name).trim();

  // Slow starts and stops give extreme paces that would squash the chart, so
  // the pace axis follows the 2nd to 98th percentile of the data instead.
  const percentileRange = (values) => {
    const sorted = values.slice().sort((a, b) => a - b);
    const lo = sorted[Math.floor((sorted.length - 1) * 0.02)];
    const hi = sorted[Math.ceil((sorted.length - 1) * 0.98)];
    const pad = (hi - lo) * 0.05 || 1;
    return { min: lo - pad, max: hi + pad };
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
      borderColor: COLORS[i % COLORS.length],
      backgroundColor: COLORS[i % COLORS.length],
      borderWidth: 1.5,
      pointRadius: 0,
      pointHoverRadius: 3,
      tension: 0.25,
      spanGaps: false,
    }));

    const values = datasets.flatMap((d) => d.data.map((p) => p.y)).filter((y) => y !== null);
    if (values.length === 0) return false; // nothing recorded for this metric

    const y = { reverse: !!metric.reverse, ticks: { callback: (v) => metric.format(v) }, grid: { color: cssVar('--line') } };
    if (canvas.dataset.metric === 'pace') Object.assign(y, percentileRange(values));

    new Chart(canvas, {
      type: 'line',
      data: { datasets },
      options: {
        parsing: false,
        animation: false,
        maintainAspectRatio: false,
        interaction: { mode: 'nearest', axis: 'x', intersect: false },
        scales: {
          x: { type: 'linear', title: { display: true, text: 'Distance (km)' }, grid: { color: cssVar('--line') } },
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
