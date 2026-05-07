// bulk.js - bulk-select bar. Tracks a Set of selected task UUIDs across
// HTMX swaps via the task-id data attribute on each row. Wires Space, *,
// Esc keybindings (Space dispatches taskwarrior:focus-row to keys.js so
// the focus and selection don't diverge).
// ─── Bulk-select (v1) ───────────────────────────────────────────────────
// Tick checkboxes on multiple rows -> bulk-mark-done or bulk-delete via the
// action bar at the top of the list. Selection state survives the 30 s poll
// because htmx:afterSwap re-ticks rows whose ids are still present.
//
// Sharing focus state with the existing keybinding block: that block applies
// the `.row-focused` class to the focused <li>; the row's checkbox is a
// descendant, so `.row-focused input.task-select` finds it without exposing
// any internal state from the existing IIFE.
(function () {
  const selected = new Set();

  function bar()        { return document.getElementById('bulk-bar'); }
  function countEl()    { return document.getElementById('bulk-count'); }
  function hiddenInput(){ return document.getElementById('bulk-ids'); }
  function doneBtn()    { return document.getElementById('bulk-done'); }
  function deleteBtn()  { return document.getElementById('bulk-delete'); }
  function clearBtn()   { return document.getElementById('bulk-clear'); }

  function checkboxes() {
    return Array.from(document.querySelectorAll('input.task-select'));
  }

  function syncDOM() {
    const n = selected.size;
    const c = countEl();
    if (c) c.textContent = String(n);
    const h = hiddenInput();
    if (h) h.value = Array.from(selected).join(',');
    const b = bar();
    if (b) {
      if (n > 0) b.removeAttribute('hidden');
      else b.setAttribute('hidden', '');
    }
    const d = doneBtn();
    if (d) d.setAttribute('hx-confirm', 'Mark ' + n + ' task' + (n === 1 ? '' : 's') + ' done?');
    const x = deleteBtn();
    if (x) x.setAttribute('hx-confirm', 'Delete ' + n + ' task' + (n === 1 ? '' : 's') + '?');
  }

  function applySelectionToCheckboxes() {
    const present = new Set();
    checkboxes().forEach(function (cb) {
      const id = cb.dataset.taskId;
      if (!id) return;
      present.add(id);
      cb.checked = selected.has(id);
    });
    // Drop ids whose row is no longer rendered (e.g. task completed elsewhere).
    Array.from(selected).forEach(function (id) {
      if (!present.has(id)) selected.delete(id);
    });
  }

  function clearSelection() {
    selected.clear();
    checkboxes().forEach(function (cb) { cb.checked = false; });
    syncDOM();
  }

  function toggleId(id, on) {
    if (!id) return;
    if (on) selected.add(id);
    else selected.delete(id);
    syncDOM();
  }

  // Checkbox change events bubble; one delegated listener handles all rows
  // (including those swapped in by HTMX after the initial render).
  document.addEventListener('change', function (e) {
    const t = e.target;
    if (!(t instanceof HTMLInputElement)) return;
    if (!t.classList.contains('task-select')) return;
    toggleId(t.dataset.taskId, t.checked);
  });

  // After every HTMX swap (poll, write-then-refresh), re-tick checkboxes for
  // ids still in the selection set and prune any that are gone.
  document.body.addEventListener('htmx:afterSwap', function () {
    applySelectionToCheckboxes();
    syncDOM();
  });

  // Wire the bar's static buttons. They live in the layout, not the swapped
  // partial, so a one-time DOMContentLoaded hookup is enough.
  function wireBar() {
    const c = clearBtn();
    if (c && !c.dataset.wired) {
      c.dataset.wired = '1';
      c.addEventListener('click', clearSelection);
    }
    // After a successful bulk POST, the server returns HX-Trigger: refresh.
    // The rows refresh and applySelectionToCheckboxes drops dead ids; but
    // for a clean UX, clear the selection entirely once the action fires.
    const d = doneBtn();
    if (d && !d.dataset.wired) {
      d.dataset.wired = '1';
      d.addEventListener('htmx:afterRequest', function (evt) {
        if (evt.detail && evt.detail.successful) clearSelection();
      });
    }
    const x = deleteBtn();
    if (x && !x.dataset.wired) {
      x.dataset.wired = '1';
      x.addEventListener('htmx:afterRequest', function (evt) {
        if (evt.detail && evt.detail.successful) clearSelection();
      });
    }
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wireBar);
  } else {
    wireBar();
  }

  function isTypingTarget(a) {
    return a && (a.tagName === 'INPUT' || a.tagName === 'TEXTAREA' || a.isContentEditable);
  }
  function dialogOpen() {
    return !!document.querySelector('dialog[open]');
  }

  // Keybindings: Space toggles the focused row's checkbox; '*' selects all
  // visible rows; Esc clears the selection (only when no dialog is open, so
  // we don't fight the native dialog Esc-to-close).
  document.addEventListener('keydown', function (e) {
    if (isTypingTarget(document.activeElement)) return;
    if (e.metaKey || e.ctrlKey || e.altKey) return;

    if (e.key === ' ' || e.code === 'Space') {
      if (dialogOpen()) return;
      // Prefer the focused row's checkbox; fall back to the first visible
      // one. When the fallback fires (no row currently focused) we also
      // dispatch `taskwarrior:focus-row` so the row-focus IIFE moves focus
      // there - otherwise the focused row and the just-selected row would
      // diverge silently and subsequent j/k would feel sticky.
      let cb = document.querySelector('.row-focused input.task-select');
      if (!cb) {
        cb = document.querySelector('input.task-select');
        if (cb) {
          const li = cb.closest('li[id^="task-"]');
          if (li) {
            li.dispatchEvent(new CustomEvent('taskwarrior:focus-row', { bubbles: true }));
          }
        }
      }
      if (!cb) return;
      e.preventDefault();
      cb.checked = !cb.checked;
      toggleId(cb.dataset.taskId, cb.checked);
      return;
    }
    if (e.key === '*') {
      if (dialogOpen()) return;
      e.preventDefault();
      checkboxes().forEach(function (cb) {
        cb.checked = true;
        if (cb.dataset.taskId) selected.add(cb.dataset.taskId);
      });
      syncDOM();
      return;
    }
    if (e.key === 'Escape') {
      // Only act when no dialog is open - the native dialog already handles
      // Esc-to-close, and we mustn't double-up.
      if (dialogOpen()) return;
      if (selected.size === 0) return;
      e.preventDefault();
      clearSelection();
      return;
    }
  });
})();
