// Copies commands, and while a scan runs, follows its progress and reloads
// the page when it is done. Everything else works without this script.
(() => {
  const still = matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (still) {
    document.querySelectorAll('animateMotion').forEach((a) => a.remove());
  }

  document.addEventListener('click', async (e) => {
    const button = e.target.closest('[data-copy]');
    if (!button) return;
    try {
      await navigator.clipboard.writeText(button.dataset.copy);
      button.textContent = 'Copied';
    } catch {
      button.textContent = 'Select it';
    }
    setTimeout(() => { button.textContent = 'Copy'; }, 1600);
  });

  if (!document.body.hasAttribute('data-scanning')) return;
  const stream = new EventSource('/events');
  stream.addEventListener('stage', (e) => {
    const row = JSON.parse(e.data);
    const li = document.querySelector(`[data-stage="${row.stage}"]`);
    if (!li) return;
    li.dataset.state = row.state;
    li.querySelector('.note').textContent = row.note;
  });
  stream.addEventListener('done', () => {
    stream.close();
    location.reload();
  });
})();
