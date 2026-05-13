// keys.js - keyboard navigation: j/k move row focus, Enter edit, x done, n add,
// ? help, Space (in bulk.js). Listens on taskwarrior:focus-row from bulk.js
// to promote a fallback-toggled row to focused.
// ─── Row focus + keybindings (v1) ───────────────────────────────────────
// Provides keyboard-only navigation: j/k move row focus, Enter opens edit,
// x marks done, ? shows the help dialog. The 'n' add-modal binding lives
// here too. Focus is tracked by the <li>'s id (UUID-based, stable across
// HTMX swaps); we restore it on htmx:afterSwap so polling doesn't lose it.
(function () {
  const FOCUS_CLASS = 'row-focused';

  let focusedId = null; // e.g. "task-<uuid>"; null = nothing focused yet.

  function rows() {
    return Array.from(document.querySelectorAll('#task-list li[id^="task-"]'));
  }

  function clearFocus() {
    document.querySelectorAll('.' + FOCUS_CLASS).forEach(function (el) {
      el.classList.remove(FOCUS_CLASS);
    });
  }

  function setFocus(li) {
    if (!li) return;
    clearFocus();
    li.classList.add(FOCUS_CLASS);
    focusedId = li.id;
    if (typeof li.scrollIntoView === 'function') {
      li.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    }
  }

  function focusedRow() {
    if (!focusedId) return null;
    return document.getElementById(focusedId);
  }

  function moveFocus(delta) {
    const list = rows();
    if (list.length === 0) return;
    const current = focusedRow();
    let idx = current ? list.indexOf(current) : -1;
    if (idx === -1) {
      idx = delta > 0 ? 0 : list.length - 1;
    } else {
      idx = Math.max(0, Math.min(list.length - 1, idx + delta));
    }
    setFocus(list[idx]);
  }

  // After every HTMX swap (incl. the 30s poll) try to keep the same row
  // focused. If the task is gone, fall back to the first row.
  document.body.addEventListener('htmx:afterSwap', function () {
    if (!focusedId) return;
    const same = document.getElementById(focusedId);
    if (same) {
      same.classList.add(FOCUS_CLASS);
      return;
    }
    const list = rows();
    if (list.length > 0) setFocus(list[0]);
    else focusedId = null;
  });

  // External focus-set channel: bulk-select Space's fallback dispatches
  // `taskwarrior:focus-row` on the row it just toggled so we promote it
  // to focused and j/k continues from there.
  document.addEventListener('taskwarrior:focus-row', function (evt) {
    const li = evt.target;
    if (li && li.matches && li.matches('li[id^="task-"]')) setFocus(li);
  });

  function isTypingTarget(a) {
    return a && (a.tagName === 'INPUT' || a.tagName === 'TEXTAREA' || a.isContentEditable);
  }

  function dialogOpen() {
    return !!document.querySelector('dialog[open]');
  }

  document.addEventListener('keydown', function (e) {
    if (isTypingTarget(document.activeElement)) return;
    if (dialogOpen()) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;

    switch (e.key) {
      case 'n':
        if (!window.htmx) return;
        e.preventDefault();
        window.htmx.ajax('GET', '/forms/add', '#modal');
        return;
      case 'j':
        e.preventDefault();
        moveFocus(1);
        return;
      case 'k':
        e.preventDefault();
        moveFocus(-1);
        return;
      case 'Enter': {
        const li = focusedRow();
        if (!li) return;
        const editBtn = li.querySelector('.row-action-edit');
        if (editBtn) {
          e.preventDefault();
          editBtn.click();
        }
        return;
      }
      case 'x': {
        const li = focusedRow();
        if (!li) return;
        const doneBtn = li.querySelector('.row-action-done');
        if (doneBtn) {
          e.preventDefault();
          doneBtn.click();
        }
        return;
      }
      case '?': {
        const dlg = document.getElementById('help-dialog');
        if (!dlg) return;
        e.preventDefault();
        dlg.showModal();
        return;
      }
    }
  });
})();
