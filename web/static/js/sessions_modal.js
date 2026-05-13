// sessions_modal.js - drives the retroactive time-tracking editor.
//
// The dialog is server-rendered as a stacked <dialog id="sessions-modal">
// swapped into #sessions-modal-slot via HTMX. This file owns:
//   - opening the dialog on htmx:afterSwap (target === slot id)
//   - the Add / Delete row buttons (delegated clicks)
//   - the Cancel / × dismiss buttons (delegated)
//   - the Save submit handler: collects rows, validates, PUTs the
//     delta payload {edits, creates, deletes} to /tasks/{id}/intervals;
//     on 204 closes + fires body refresh, on 422 renders the conflict
//     panel from the structured JSON response.
//   - the "Show all" toggle: re-fetches the dialog without ?day=...
//
// The dialog uses native <dialog>.showModal() so focus trap, Esc, backdrop
// click, and rest-of-page inert are browser-native - no JS workaround.
(function () {
  // Open the dialog after HTMX swaps it into #sessions-modal. Using the
  // standard htmx:afterSwap event (which always bubbles to body) is more
  // reliable than relying on the custom HX-Trigger event - that one only
  // fires on the request initiator and may not bubble depending on the
  // HTMX version.
  document.body.addEventListener('htmx:afterSwap', function (evt) {
    var target = evt.detail && evt.detail.target;
    // The slot wrapper is #sessions-modal-slot; the dialog inside it is
    // #sessions-modal. The afterSwap event's `target` is the SWAP target
    // (the slot, since hx-target="#sessions-modal-slot"). Match on slot
    // id, then look up the dialog inside to call showModal().
    if (!target || target.id !== 'sessions-modal-slot') return;
    var dlg = document.getElementById('sessions-modal');
    if (!dlg) return;
    if (!dlg.open) dlg.showModal();
    initFlatpickr(dlg);
  });

  // "Earlier days" page fragments swap in fresh session rows mid-
  // modal-life. Re-init flatpickr on the newly-rendered slice each
  // time an htmx swap settles inside the sessions modal.
  document.body.addEventListener('htmx:afterSettle', function (evt) {
    var dlg = document.getElementById('sessions-modal');
    if (!dlg || !dlg.open) return;
    var target = evt.detail && evt.detail.target;
    if (!target || !dlg.contains(target)) return;
    initFlatpickr(target);
  });

  // Click on the dialog backdrop closes it (target === dialog when the
  // user clicks the ::backdrop pseudo-element). Same pattern as mobile_nav.
  document.addEventListener('click', function (e) {
    var target = e.target;
    if (!(target instanceof Element)) return;

    // Dismiss buttons (Cancel + × in header).
    if (target.closest('[data-dismiss-sessions]')) {
      var dlg = document.getElementById('sessions-modal');
      if (dlg && dlg.open) dlg.close();
      return;
    }

    // Backdrop click on the dialog itself. Coordinate-test against
    // the dialog's box because target===dialog also fires when
    // mousedown and mouseup land on different descendants (e.g. tap
    // input, flatpickr opens between down/up, mouseup hits the
    // picker) - the browser dispatches click on the common ancestor.
    if (target.id === 'sessions-modal') {
      var r = target.getBoundingClientRect();
      if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) {
        target.close();
      }
      return;
    }

    // Add a fresh row into the STAGING area ([data-sessions-new]) which
    // lives outside the scrolling history. This keeps the user's scroll
    // position intact: they may be inspecting a specific past date when
    // they click Add, and yanking them away from it would be hostile.
    // New rows accumulate next to the Add button - the cause-and-effect
    // is right there in their viewport.
    //
    // On Save the submit handler grabs ALL [data-session-row] elements
    // in the form regardless of which container they live in, so the
    // staging rows submit alongside the history rows automatically.
    if (target.closest('[data-sessions-add]')) {
      var staging = document.querySelector('[data-sessions-new]');
      if (!staging) return;
      var row = newSessionRow();
      if (!row) return;
      staging.appendChild(row);
      initFlatpickr(row);
      setTimeout(function () {
        var s = row.querySelector('[data-session-start]');
        if (s && typeof s.focus === 'function') s.focus();
      }, 0);
      return;
    }

    // Delete a row. Two cases:
    //
    //   - Staging row (in [data-sessions-new], no data-original-start):
    //     the user just added it this turn. Remove from DOM; the diff
    //     submission ignores it.
    //
    //   - Existing row (data-original-start present): the row represents
    //     an on-disk pair. We do NOT remove it from the DOM, because the
    //     delta submit needs the (data-original-start, data-original-end)
    //     identity to send a `deletes` entry to the BE. Instead we mark
    //     the row with [data-pending-delete] and hide it visually. On
    //     save, the submit handler reads the marked rows, builds the
    //     deletes array, and the rendered list rebuilds from the BE
    //     response (which will no longer contain the deleted pair).
    //
    // Confirm modal is shown only for the existing-row case - misclick
    // protection where information would be lost. Staging-row removal
    // is trivially recoverable (click Add again).
    var delBtn = target.closest('[data-session-delete]');
    if (delBtn) {
      var row = delBtn.closest('[data-session-row]');
      if (!row) return;
      var hasOriginal = !!row.getAttribute('data-original-start');
      if (!hasOriginal) {
        // Brand new row - just remove. No confirm, no diff entry.
        row.remove();
        return;
      }
      appConfirm('Delete this time entry?', function () {
        row.setAttribute('data-pending-delete', '');
        row.style.display = 'none';
      });
      return;
    }

    // "Show all" toggle is wired via inline HTMX attributes on the button
    // itself (sessionsDayFilterBar in sessions_modal.templ): hx-get +
    // hx-target="#sessions-modal-slot". The standard afterSwap handler
    // above reopens the freshly-swapped dialog. No JS branch needed here.
  });

  // Submit: build the diff payload {edits, creates, deletes} and PUT
  // it. The payload shape is intentionally narrow:
  //
  //   - deletes: rows the user marked with [data-pending-delete] (the
  //     delete button hides them but leaves the row in the DOM so we
  //     can read its data-original-* identity here).
  //   - creates: rows with no data-original-start (those are brand-new
  //     rows added via "+ Add entry").
  //   - edits: rows with a data-original-start whose current Start or
  //     End values differ from the rendered original. Unchanged
  //     existing rows are dropped from the payload entirely (no-op).
  //
  // Rows on UNLOADED pages aren't in the DOM and therefore aren't in
  // the payload at all - "absence = leave alone" is the invariant
  // that makes partial-view saves non-destructive.
  document.addEventListener('submit', function (e) {
    var form = e.target;
    if (!form.matches || !form.matches('[data-sessions-form]')) return;
    e.preventDefault();

    var taskId = form.dataset.taskId;
    var isActive = form.dataset.taskActive === '1';
    if (!taskId) return;

    var rows = form.querySelectorAll('[data-session-row]');
    var edits = [];
    var creates = [];
    var deletes = [];
    var clientError = null;
    var openCount = 0;
    rows.forEach(function (row) {
      row.classList.remove('session-row-error');

      // Rows hidden behind a conflict-panel duplicate are superseded
      // by their duplicate (which lives in [data-sessions-conflicts]
      // and has fresh user input). Skipping here keeps the
      // submission from counting them twice.
      if (row.hasAttribute('data-conflict-hidden')) return;

      if (row.hasAttribute('data-pending-delete')) {
        var origStart = row.getAttribute('data-original-start');
        if (origStart) {
          deletes.push({
            originalStart: origStart,
            originalEnd: row.getAttribute('data-original-end') || null,
          });
        }
        return;
      }

      var startEl = row.querySelector('[data-session-start]');
      var endEl = row.querySelector('[data-session-end]');
      if (!startEl || !startEl.value) {
        clientError = clientError || 'Every entry needs a start time';
        row.classList.add('session-row-error');
        return;
      }
      var startISO = new Date(startEl.value).toISOString();
      var endISO = null;
      if (endEl && !endEl.disabled && endEl.value) {
        endISO = new Date(endEl.value).toISOString();
        // Allow end == start: the datetime-local input is minute-
        // granular, so a real sub-minute interval round-trips as
        // end == start. Block strict less-than only.
        if (endISO < startISO) {
          clientError = clientError || 'End must not be earlier than start';
          row.classList.add('session-row-error');
        }
      } else if (endEl && endEl.disabled) {
        // Active (open) interval - leave end null.
        openCount++;
      }

      var origStart = row.getAttribute('data-original-start');
      if (!origStart) {
        // New row (no on-disk identity yet) -> create.
        creates.push({ start: startISO, end: endISO });
        return;
      }

      // Existing row. Compare current values to original (both
      // normalised through Date for cross-format equality) to decide
      // whether this is an edit or an unchanged no-op.
      var origEndAttr = row.getAttribute('data-original-end') || '';
      var origStartISO = new Date(origStart).toISOString();
      var origEndISO = origEndAttr ? new Date(origEndAttr).toISOString() : null;
      if (origStartISO === startISO && origEndISO === endISO) {
        // No change - drop entirely from the payload.
        return;
      }
      edits.push({
        originalStart: origStart,
        originalEnd: origEndAttr || null,
        start: startISO,
        end: endISO,
      });
    });

    if (openCount > 0 && !isActive) {
      clientError = clientError || 'An open entry requires the task to be active';
    }

    var errPanel = form.querySelector('[data-sessions-error]');
    if (clientError) {
      if (errPanel) errPanel.textContent = clientError;
      return;
    }
    if (errPanel) errPanel.textContent = '';

    // Empty diff (user clicked Save without changing anything) is a
    // no-op. Skip the network round-trip and close the dialog.
    if (edits.length === 0 && creates.length === 0 && deletes.length === 0) {
      var dlg = document.getElementById('sessions-modal');
      if (dlg && dlg.open) dlg.close();
      return;
    }

    // CSRF token: prefer the hidden _csrf input rendered inside the
    // form (single source of truth, matches modal_edit's shape), with
    // the meta tag as a defensive fallback.
    var csrfInput = form.querySelector('input[name="_csrf"]');
    var csrf = (csrfInput && csrfInput.value)
      || (document.querySelector('meta[name="csrf-token"]') || {}).content
      || '';

    // Clear any conflict-panel state left over from a previous attempt
    // before sending the new request. If the new response brings back
    // conflicts, the renderer rebuilds the panel from scratch; if it
    // brings 204, we close the modal anyway.
    clearConflictState(form);

    fetch('/tasks/' + taskId + '/intervals', {
      method: 'PUT',
      headers: {
        'Content-Type': 'application/json',
        'X-CSRF-Token': csrf,
      },
      body: JSON.stringify({ edits: edits, creates: creates, deletes: deletes }),
    }).then(function (resp) {
      if (resp.status === 204) {
        // creates and deletes are exactly the rows that change the
        // total - edits hold the count constant. Patch the trigger
        // label in the underlying edit modal in place so the user
        // doesn't see a stale count after dismissing the dialog.
        updateTrackedTimeLabel(creates.length - deletes.length);
        var dlg = document.getElementById('sessions-modal');
        if (dlg && dlg.open) dlg.close();
        if (window.htmx) window.htmx.trigger(document.body, 'refresh');
        return;
      }
      return resp.json().then(function (data) {
        if (errPanel) errPanel.textContent = data.error || ('Save failed: HTTP ' + resp.status);
        if (data.conflicts && data.conflicts.length > 0) {
          renderConflicts(form, data.conflicts);
        }
      }, function () {
        if (errPanel) errPanel.textContent = 'Save failed: HTTP ' + resp.status;
      });
    }).catch(function (err) {
      if (errPanel) errPanel.textContent = 'Network error: ' + err.message;
    });
  });

  // appConfirm shows the app's styled #confirm-dialog (defined in
  // layout.templ, shared with core_confirm.js's HTMX hx-confirm
  // handler). The delete-row action is a pure-JS operation - no
  // request to confirm-and-issue - so we open the same dialog
  // manually and run onConfirm if the user picks confirm. Falls back
  // to window.confirm if the styled dialog isn't on the page
  // (defensive against partial renders).
  function appConfirm(question, onConfirm) {
    var dialog = document.getElementById('confirm-dialog');
    var message = document.getElementById('confirm-message');
    if (!dialog || !message) {
      if (window.confirm(question)) onConfirm();
      return;
    }
    message.textContent = question;
    dialog.returnValue = '';
    dialog.showModal();
    dialog.addEventListener('close', function handler() {
      dialog.removeEventListener('close', handler);
      if (dialog.returnValue === 'confirm') onConfirm();
    });
  }

  // renderConflicts builds the conflict panel from the BE's structured
  // response. Each pair becomes a banner-style group with two editable
  // mini-rows. The corresponding source rows in the main list (or the
  // staging area for new creates) are marked data-conflict-hidden and
  // hidden via display:none so the conflict-panel duplicate is the
  // single source of truth for that identity in the next submission.
  function renderConflicts(form, conflicts) {
    var panel = form.querySelector('[data-sessions-conflicts]');
    if (!panel) return;
    panel.innerHTML = '';
    conflicts.forEach(function (pair) {
      var pairEl = document.createElement('div');
      pairEl.className = 'rounded border border-red-300 bg-red-50/60 p-3 dark:border-red-800 dark:bg-red-950/30';
      var label = document.createElement('p');
      label.className = 'mb-2 text-xs font-medium text-red-700 dark:text-red-300';
      label.textContent = 'These entries overlap:';
      pairEl.appendChild(label);
      pair.rows.forEach(function (rowData) {
        hideSourceRow(form, rowData);
        var conflictRow = buildConflictRow(rowData);
        if (conflictRow) pairEl.appendChild(conflictRow);
      });
      panel.appendChild(pairEl);
    });
    if (typeof panel.scrollIntoView === 'function') {
      panel.scrollIntoView({behavior: 'smooth', block: 'start'});
    }
  }

  // clearConflictState resets any conflict-panel UI from a previous
  // save attempt: empties the panel, unhides any source rows that
  // were hidden during the prior pass. Run at the start of each
  // submit so the next response renders against a clean slate.
  function clearConflictState(form) {
    var panel = form.querySelector('[data-sessions-conflicts]');
    if (panel) panel.innerHTML = '';
    form.querySelectorAll('[data-conflict-hidden]').forEach(function (row) {
      row.removeAttribute('data-conflict-hidden');
      row.style.display = '';
    });
  }

  // hideSourceRow finds the main-list / staging-area row that matches
  // the conflict row's identity and hides it. Without hiding, the
  // submit pipeline would iterate BOTH the source and the conflict-
  // panel duplicate - producing duplicate creates or double edits.
  function hideSourceRow(form, rowData) {
    var rows = form.querySelectorAll('[data-session-row]');
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i];
      if (row.hasAttribute('data-conflict-hidden')) continue;
      if (row.closest('[data-sessions-conflicts]')) continue;
      if (rowData.kind === 'create') {
        if (row.getAttribute('data-original-start')) continue;
        if (matchesCreateRow(row, rowData)) {
          markHidden(row);
          return;
        }
      } else {
        if (matchesExistingRow(row, rowData)) {
          markHidden(row);
          return;
        }
      }
    }
  }

  function markHidden(row) {
    row.setAttribute('data-conflict-hidden', '');
    row.style.display = 'none';
  }

  // Both inputs are validated via Date parsing before any property access;
  // an unparseable input flows back as NaN.getTime() which never matches.
  function sameInstant(a, b) {
    var da = new Date(a).getTime();
    var db = new Date(b).getTime();
    return !isNaN(da) && !isNaN(db) && da === db;
  }

  function matchesCreateRow(row, rowData) {
    var s = row.querySelector('[data-session-start]');
    var e = row.querySelector('[data-session-end]');
    if (!s || !s.value) return false;
    if (!sameInstant(s.value, rowData.currentStart)) return false;
    if (rowData.currentEnd) {
      if (!e || e.disabled || !e.value) return false;
      return sameInstant(e.value, rowData.currentEnd);
    }
    return !(e && !e.disabled && e.value);
  }

  function matchesExistingRow(row, rowData) {
    var os = row.getAttribute('data-original-start') || '';
    if (!os || !rowData.originalStart) return false;
    if (!sameInstant(os, rowData.originalStart)) return false;
    var oe = row.getAttribute('data-original-end') || '';
    if (rowData.originalEnd) {
      return oe && sameInstant(oe, rowData.originalEnd);
    }
    return !oe;
  }

  function buildConflictRow(rowData) {
    var row = newSessionRow();
    if (!row) return null;
    if (rowData.originalStart) row.setAttribute('data-original-start', rowData.originalStart);
    if (rowData.originalEnd) row.setAttribute('data-original-end', rowData.originalEnd);
    // Pre-fill the hidden ISO inputs BEFORE flatpickr attaches:
    // flatpickr seeds the alt input from the original input's
    // value during attach, so this is the cleanest way to get
    // both inputs in sync without juggling fp.setDate after.
    var startInput = row.querySelector('[data-session-start]');
    var endInput = row.querySelector('[data-session-end]');
    if (startInput) startInput.value = utcToLocalInput(rowData.currentStart);
    if (endInput) {
      if (rowData.currentEnd) {
        endInput.value = utcToLocalInput(rowData.currentEnd);
      } else {
        endInput.disabled = true;
        endInput.placeholder = 'Active';
      }
    }
    initFlatpickr(row);
    return row;
  }

  // Live-correct flatpickr time inputs; otherwise invalid values
  // stay visible until blur.
  document.body.addEventListener('input', function (e) {
    var t = e.target;
    if (!t || !t.matches) return;
    var rolloverAt;
    if (t.matches('.flatpickr-hour')) rolloverAt = 24;
    else if (t.matches('.flatpickr-minute')) rolloverAt = 60;
    else return;
    var v = parseInt(t.value, 10);
    if (isNaN(v)) return;
    if (v < 0) t.value = '0';
    else if (v === rolloverAt) t.value = '0';
    else if (v > rolloverAt) t.value = String(rolloverAt - 1);
  });

  // updateTrackedTimeLabel patches the "Tracked time (N)" / "Add
  // time entry" affordance on the parent edit modal after a
  // successful save. Avoids an extra server round-trip just to
  // re-render the trigger button.
  function updateTrackedTimeLabel(delta) {
    if (!delta) return;
    var label = document.querySelector('[data-tracked-time-label]');
    if (!label) return;
    var match = /Tracked time \((\d+)\)/.exec(label.textContent || '');
    var current = match ? parseInt(match[1], 10) : 0;
    var next = current + delta;
    if (next <= 0) {
      label.textContent = 'Add time entry';
    } else {
      label.textContent = 'Tracked time (' + next + ')';
    }
  }

  // utcToLocalInput converts an RFC3339 UTC timestamp to the local
  // "YYYY-MM-DDTHH:MM" shape datetime-local inputs expect. Matches
  // the server-side localDateTimeInput format. Mobile reads this
  // directly via the native input type; desktop flatpickr reads it
  // via dateFormat: 'Y-m-dTH:i'.
  function utcToLocalInput(iso) {
    var d = new Date(iso);
    if (isNaN(d.getTime())) return '';
    var pad = function (n) { return n < 10 ? '0' + n : String(n); };
    return d.getFullYear() + '-' + pad(d.getMonth() + 1) + '-' + pad(d.getDate()) +
      'T' + pad(d.getHours()) + ':' + pad(d.getMinutes());
  }

  // initFlatpickr attaches flatpickr (with the confirmDate plugin's
  // explicit OK button) to every Start/End wrapper under the given
  // root using wrap mode: clicking the input OR the calendar icon
  // both open the picker. Idempotent - skips wrappers that already
  // have a flatpickr instance, and disabled inputs (active-interval
  // End slot). Picker reads/writes "Y-m-dTH:i" matching server-side
  // localDateTimeInput / utcToLocalInput so the submit handler's
  // `new Date(input.value).toISOString()` parsing keeps working.
  function initFlatpickr(root) {
    if (typeof flatpickr !== 'function') return;
    var scope = root && root.querySelectorAll ? root : document;
    scope.querySelectorAll('[data-session-fp]').forEach(function (wrap) {
      if (wrap._flatpickr) return;
      var input = wrap.querySelector('[data-input]');
      if (!input || input.disabled) return;
      // Server renders datetime-local for progressive enhancement;
      // switch to text so the OS-native popup doesn't compete with
      // flatpickr's.
      if (input.type !== 'text') input.type = 'text';
      var dialog = wrap.closest('dialog');
      flatpickr(wrap, {
        wrap: true,
        enableTime: true,
        time_24hr: true,
        allowInput: true,
        dateFormat: 'Y-m-dTH:i',
        // altInput renders a sibling text input with the
        // human-readable altFormat for the user, while the
        // original input (form-submit + data-session-start)
        // stays in the parseable ISO dateFormat. Year is
        // included because session entries can span months /
        // years (the editor lists historical sessions).
        altInput: true,
        altFormat: 'D j M Y, H:i',
        altInputClass: 'w-full rounded border border-zinc-300 bg-white px-2 py-1 pr-8 text-sm text-zinc-900 focus:border-blue-500 focus:outline-none dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100',
        appendTo: dialog || document.body,
        disableMobile: true,
        onPreCalendarPosition: function () { window.tw.flatpickr.ensureRoomForPicker(input); },
        onOpen: function (_sd, _ds, fp) { fp.__valueOnOpen = fp.input.value; },
        onReady: function (_sd, _ds, fp) {
          window.tw.flatpickr.addPickerFooter(fp);
          window.tw.flatpickr.tameTimeInputs(fp);
          // flatpickr's altInput is the visible field but inherits no
          // a11y context from the hidden original input (which is what
          // the surrounding <label> wraps in markup terms). Mirror the
          // start/end identity onto the alt input so screen readers
          // hear "Session start, Tue 12 May 2026, 19:06" instead of an
          // unnamed text field.
          if (fp.altInput) {
            var label = fp.input.hasAttribute('data-session-start') ? 'Session start' : 'Session end';
            fp.altInput.setAttribute('aria-label', label);
          }
        },
      });
    });
  }

  // Clone the server-rendered <template id="session-row-template">
  // (defined in sessions_modal.templ → SessionRowTemplate) instead of
  // hand-building HTML. One source of truth for the row markup: any
  // Tailwind class or SVG-path tweak in the templ flows here for free.
  function newSessionRow() {
    var tpl = document.getElementById('session-row-template');
    if (!tpl || !tpl.content || !tpl.content.firstElementChild) return null;
    return tpl.content.firstElementChild.cloneNode(true);
  }
})();
