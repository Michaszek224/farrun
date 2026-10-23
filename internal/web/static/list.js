// Enables the "Compare selected" button only while a usable number of runs is
// ticked. Without JavaScript the button stays enabled and the server answers
// with a helpful message instead.
(() => {
  'use strict';

  const form = document.querySelector('[data-compare-form]');
  if (!form) return;

  const min = Number(form.dataset.min);
  const max = Number(form.dataset.max);
  const boxes = [...form.querySelectorAll('input[type="checkbox"]')];
  const button = form.querySelector('button[type="submit"]');
  const hint = form.querySelector('[data-compare-hint]');

  const update = () => {
    const n = boxes.filter((b) => b.checked).length;
    button.disabled = n < min || n > max;
    if (n === 0) hint.textContent = `Tick ${min} to ${max} runs to compare them.`;
    else if (n > max) hint.textContent = `${n} selected: pick at most ${max}.`;
    else if (n < min) hint.textContent = `${n} selected: pick at least ${min}.`;
    else hint.textContent = `${n} selected.`;
  };

  boxes.forEach((b) => b.addEventListener('change', update));
  update();
})();
