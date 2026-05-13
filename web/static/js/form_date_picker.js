// form_date_picker.js - attaches flatpickr to [data-fp-date] wraps
// in the add/edit task modal date fields (Due, Wait, Scheduled,
// Until). Wrap-mode contract: the wrapper has [data-fp-date], the
// input has [data-input], the icon button has [data-toggle]. Click
// on the input is NOT a picker trigger (clickOpens: false) - only
// the icon opens it - so users can still type Taskwarrior relative
// dates like "+2d" / "tomorrow" without the popup getting in the
// way. The picker writes ISO dates (Y-m-d) which TW accepts.
(function () {
  // Scroll the input's nearest scrollable ancestor (typically the
  // dialog) so the picker fits below the field. flatpickr's auto-
  // positioning only flips above when there's room above; in a
  // tight viewport with a tall dialog there's room in neither
  // direction and the popup overflows the bottom edge. Pre-scrolling
  // means flatpickr's position calculation runs against viable
  // coords. PICKER_HEIGHT is the worst-case (calendar + time bar +
  // confirm button) - over-budgeting is fine, under-budgeting is
  // what produced the original bug.
  function ensureRoomForPicker(input) {
    // Calendar + time bar + custom footer worst-case. Over-
    // budgeting is fine - we just over-scroll the dialog by a
    // few px; under-budgeting hangs the picker off the bottom.
    var PICKER_HEIGHT = 420;
    var rect = input.getBoundingClientRect();
    var below = window.innerHeight - rect.bottom;
    if (below >= PICKER_HEIGHT) return;
    var needed = PICKER_HEIGHT - below + 16;
    var el = input.parentElement;
    while (el && el !== document.documentElement) {
      var s = getComputedStyle(el);
      if (/auto|scroll/.test(s.overflowY) && el.scrollHeight > el.clientHeight) {
        el.scrollTop = Math.min(el.scrollTop + needed, el.scrollHeight - el.clientHeight);
        return;
      }
      el = el.parentElement;
    }
    window.scrollBy(0, needed);
  }
  // window.tw.flatpickr exposes the three picker helpers that
  // sessions_modal.js consumes. The two files always load together
  // (layout.templ pins script order: form_date_picker.js after
  // sessions_modal.js), so the load-order contract is enforced by
  // markup, not by guards in the consumer.
  window.tw = window.tw || {};
  window.tw.flatpickr = {
    ensureRoomForPicker: ensureRoomForPicker,
    addPickerFooter: null,
    tameTimeInputs: null,
  };

  // Appends a red-X cancel / green-tick confirm pair as a footer
  // strip inside the flatpickr calendar. Replaces the confirmDate
  // plugin's "OK ✓" button so the UI carries an explicit revert
  // affordance and a more icon-forward visual language. Cancel
  // restores the input value to whatever it was when the picker
  // opened (snapshotted by the caller's onOpen hook into
  // fp.__valueOnOpen). Idempotent - skips if a footer is already
  // attached to the same calendar.
  var CANCEL_ICON = '<svg width="22" height="22" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 5l10 10M15 5L5 15"/></svg>';
  var CONFIRM_ICON = '<svg width="22" height="22" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="3 10 8 15 17 5"/></svg>';

  function addPickerFooter(fp) {
    var cal = fp && fp.calendarContainer;
    if (!cal || cal.querySelector('.tw-fp-footer')) return;
    var footer = document.createElement('div');
    footer.className = 'tw-fp-footer';
    var cancel = pickerButton('cancel', 'Cancel', CANCEL_ICON);
    cancel.addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      if (fp.__valueOnOpen != null) {
        try { fp.setDate(fp.__valueOnOpen, false); }
        catch (_) { /* unparseable original value - skip revert */ }
      }
      fp.close();
    });
    var confirm = pickerButton('confirm', 'Confirm', CONFIRM_ICON);
    confirm.addEventListener('click', function (e) {
      e.preventDefault();
      e.stopPropagation();
      fp.close();
    });
    footer.appendChild(cancel);
    footer.appendChild(confirm);
    cal.appendChild(footer);
  }
  window.tw.flatpickr.addPickerFooter = addPickerFooter;

  function pickerButton(kind, label, iconSvg) {
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'tw-fp-' + kind;
    btn.setAttribute('aria-label', label);
    btn.title = label;
    btn.innerHTML = iconSvg;
    return btn;
  }

  // Wire each hour/minute input so iOS doesn't auto-select its
  // value on focus. The selection is what triggers the iOS Look
  // Up / Copy / Paste action sheet over the picker - placing the
  // cursor at the end of the value instead leaves the field
  // editable but unselected, so the menu never appears. The
  // contextmenu listener belt-and-braces against right-click /
  // long-press on platforms that surface their own menu from a
  // gesture rather than from the selection.
  function tameTimeInputs(fp) {
    var cal = fp && fp.calendarContainer;
    if (!cal) return;
    cal.querySelectorAll('.flatpickr-hour, .flatpickr-minute').forEach(function (input) {
      if (input.__twTamed) return;
      input.__twTamed = true;
      // Switch from type=number to type=text + inputmode=numeric.
      // Two reasons: (a) iOS auto-selects the value of a focused
      // number input, which surfaces its Look Up / Copy / Paste
      // edit menu over the picker; (b) setSelectionRange is not
      // supported on number inputs, so we cannot programmatically
      // clear that selection. Text + inputmode=numeric keeps the
      // numeric keypad on mobile and unlocks selection control.
      if (input.type === 'number') input.type = 'text';
      input.setAttribute('inputmode', 'numeric');
      input.addEventListener('focus', function () {
        // iOS applies its auto-select AFTER the focus handler
        // returns. The 0-delay task fires next and clears the
        // selection before the OS gets a chance to surface its
        // edit menu attached to that selection.
        setTimeout(function () {
          try { input.setSelectionRange(input.value.length, input.value.length); }
          catch (_) { /* defensive - selection API absent */ }
        }, 0);
      });
      // Belt-and-braces against long-press / right-click on
      // platforms that surface their own menu from a gesture
      // rather than from a text selection.
      input.addEventListener('contextmenu', function (e) { e.preventDefault(); });
    });
  }
  window.tw.flatpickr.tameTimeInputs = tameTimeInputs;

  function initAll(root) {
    if (typeof flatpickr !== 'function') return;
    var scope = root && root.querySelectorAll ? root : document;
    scope.querySelectorAll('[data-fp-date]').forEach(function (wrap) {
      if (wrap._fp) return;
      var input = wrap.querySelector('[data-input]');
      if (!input || input.disabled || input.readOnly) return;
      var dialog = wrap.closest('dialog');
      wrap._fp = flatpickr(wrap, {
        wrap: true,
        dateFormat: 'Y-m-d',
        allowInput: true,
        clickOpens: false,
        appendTo: dialog || document.body,
        disableMobile: true,
        onPreCalendarPosition: function () { ensureRoomForPicker(input); },
      });
    });
  }

  // Modals load their forms via HTMX swap; re-init on every swap so
  // freshly-rendered date inputs in the new content get a picker.
  document.body.addEventListener('htmx:afterSwap', function (evt) {
    initAll(evt.detail && evt.detail.target);
  });
  // Server-rendered first paint: cover any wraps present on page load.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', function () { initAll(document); });
  } else {
    initAll(document);
  }
})();
