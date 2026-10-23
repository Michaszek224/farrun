// Dates "decrypt": the digits start as random glyphs and settle into the real
// date, one row after another when the page loads, and again under the mouse.
// The real text is kept in an aria-label, so screen readers never hear the noise,
// and the glyphs all have the width of a digit, so nothing shifts while it runs.
(() => {
  'use strict';

  if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  const GLYPHS = '0123456789ABCDEF░▒▓#%$@&*+<>/|_=?';
  const LOAD_MS = 700;   // how long a date takes to settle when the page loads
  const HOVER_MS = 420;  // and when you point at it
  const STAGGER_MS = 28; // delay between neighbouring rows on load
  const MAX_DELAY_MS = 900;

  const targets = [...document.querySelectorAll('[data-date]')];
  if (targets.length === 0) return;

  const finalText = new Map(); // element -> the real date
  const running = new Map();   // element -> { start, duration }
  let frame = 0;

  const randomGlyph = () => GLYPHS[Math.floor(Math.random() * GLYPHS.length)];

  // Only digits are scrambled; "-", ":" and the space stay where they are.
  const render = (el, progress) => {
    const text = finalText.get(el);
    const digits = [...text].filter((c) => /\d/.test(c)).length;
    let seen = 0;
    el.textContent = [...text].map((c) => {
      if (!/\d/.test(c)) return c;
      const settleAt = (seen++ + 1) / (digits + 1); // left digits settle first
      return progress >= settleAt ? c : randomGlyph();
    }).join('');
  };

  const tick = (now) => {
    frame = 0;
    for (const [el, run] of running) {
      const progress = (now - run.start) / run.duration;
      if (progress >= 1) {
        el.textContent = finalText.get(el);
        running.delete(el);
      } else {
        render(el, Math.max(progress, 0)); // before its turn it is fully scrambled
      }
    }
    if (running.size > 0) frame = requestAnimationFrame(tick);
  };

  const decrypt = (el, delay, duration) => {
    running.set(el, { start: performance.now() + delay, duration });
    if (!frame) frame = requestAnimationFrame(tick);
  };

  targets.forEach((el, i) => {
    finalText.set(el, el.textContent);
    if (!el.hasAttribute('aria-label')) el.setAttribute('aria-label', el.textContent);

    if (el.tagName === 'A') {
      el.addEventListener('mouseenter', () => decrypt(el, 0, HOVER_MS));
      el.addEventListener('focus', () => decrypt(el, 0, HOVER_MS));
    }
    decrypt(el, Math.min(i * STAGGER_MS, MAX_DELAY_MS), LOAD_MS);
  });
})();
