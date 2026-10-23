// "/" jumps to the filter box, like in a terminal pager or on GitHub; Escape leaves it.
(() => {
  'use strict';

  const input = document.querySelector('form.query input[name="q"]');
  if (!input) return;

  const typingSomewhere = () => {
    const el = document.activeElement;
    return !!el && (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName) || el.isContentEditable);
  };

  document.addEventListener('keydown', (e) => {
    if (e.key === '/' && !e.ctrlKey && !e.metaKey && !e.altKey && !typingSomewhere()) {
      e.preventDefault(); // otherwise the "/" would be typed into the box
      input.focus();
      input.select();
    } else if (e.key === 'Escape' && document.activeElement === input) {
      input.blur();
    }
  });
})();
