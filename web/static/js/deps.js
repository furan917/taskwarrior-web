// deps.js - dependency picker (multi-select) inside the task edit/add modal.
// Listens for autocomplete:select from autocomplete.js, preventDefaults the
// default value-mutation, and adds a pill carrying data-uuid. Hidden field
// is regenerated from the live DOM after each mutation.
// ─── Dependency picker (v5) ─────────────────────────────────────────────
// The edit/add modals carry a multi-select dependency picker rendered by
// dep_picker.templ as <div data-dep-picker> ... </div>. Inside that div:
//
//   - `.dep-pills`   - flex container holding zero or more <span class="dep-pill"
//                      data-uuid="...">. Each pill has a `.dep-pill-remove`
//                      button.
//   - `.dep-input`   - text input wired to the themed autocomplete IIFE
//                      (mode="deps"). Each <li role="option"> in the
//                      adjacent [data-ac-list] carries data-ac-value=<uuid>
//                      and data-ac-text=<description>.
//   - `.dep-hidden`  - <input type="hidden" name="depends">. We keep its value
//                      synced to the comma-joined uuid list of all current
//                      pills so form submission carries the right payload.
//
// We listen for:
//   - autocomplete:select on .dep-input -> consume the {value=uuid, text=desc}
//     from the autocomplete IIFE, preventDefault its default value-mutation,
//     append a pill, clear the input.
//   - keydown Enter on .dep-input where the autocomplete had no highlight ->
//     fallback for "user typed an exact description and pressed Enter without
//     arrowing down to highlight it"; resolves against the rendered options.
//   - click on .dep-pill-remove (delegated) -> remove the pill, sync hidden.
//
// The hidden field is regenerated from the live DOM after each mutation so
// stale state can't leak in (e.g. an htmx swap mid-edit).
(function () {
  function pickerOf(el) {
    return el && el.closest && el.closest('[data-dep-picker]');
  }
  function syncHidden(picker) {
    if (!picker) return;
    var hidden = picker.querySelector('.dep-hidden');
    if (!hidden) return;
    var uuids = [];
    picker.querySelectorAll('.dep-pill').forEach(function (pill) {
      var u = pill.getAttribute('data-uuid');
      if (u) uuids.push(u);
    });
    hidden.value = uuids.join(',');
  }
  function existingUUIDs(picker) {
    var set = new Set();
    if (!picker) return set;
    picker.querySelectorAll('.dep-pill').forEach(function (pill) {
      var u = pill.getAttribute('data-uuid');
      if (u) set.add(u);
    });
    return set;
  }
  function resolveTyped(picker, typed) {
    // Matches typed text against the picker's local [data-ac-list] options.
    // First exact-text match wins, then case-insensitive equality. Anything
    // else returns null. Replaces the previous datalist-scoped resolver.
    var listed = picker.querySelectorAll('[data-ac-list] [role="option"]');
    if (!listed || listed.length === 0) return null;
    var lower = typed.toLowerCase();
    for (var i = 0; i < listed.length; i++) {
      var opt = listed[i];
      if (opt.getAttribute('data-ac-text') === typed) {
        return { uuid: opt.getAttribute('data-ac-value'), label: typed };
      }
    }
    for (var j = 0; j < listed.length; j++) {
      var o2 = listed[j];
      var t = o2.getAttribute('data-ac-text');
      if (t && t.toLowerCase() === lower) {
        return { uuid: o2.getAttribute('data-ac-value'), label: t };
      }
    }
    return null;
  }
  function makePill(uuid, label) {
    var span = document.createElement('span');
    span.className = 'dep-pill inline-flex items-center gap-1 rounded bg-zinc-100 px-2 py-0.5 text-xs text-zinc-700 dark:bg-zinc-700 dark:text-zinc-200';
    span.setAttribute('data-uuid', uuid);
    var labelEl = document.createElement('span');
    labelEl.className = 'dep-pill-label';
    labelEl.textContent = label || uuid;
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'dep-pill-remove text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-100';
    btn.setAttribute('aria-label', 'Remove dependency');
    btn.title = 'Remove';
    btn.textContent = '×';
    span.appendChild(labelEl);
    span.appendChild(btn);
    return span;
  }
  function addPill(picker, uuid, label) {
    if (!uuid) return;
    if (existingUUIDs(picker).has(uuid)) return;
    var pillsHost = picker.querySelector('.dep-pills');
    if (!pillsHost) return;
    pillsHost.appendChild(makePill(uuid, label));
    syncHidden(picker);
  }
  // Primary path: the autocomplete IIFE picked an option and dispatched
  // autocomplete:select with {value=uuid, text=description}. We preventDefault
  // its default value-mutation, then append the pill ourselves.
  document.addEventListener('autocomplete:select', function (e) {
    var t = e.target;
    if (!(t instanceof HTMLInputElement)) return;
    if (!t.classList.contains('dep-input')) return;
    var picker = pickerOf(t);
    if (!picker) return;
    e.preventDefault();
    addPill(picker, e.detail && e.detail.value, e.detail && e.detail.text);
    t.value = '';
  });
  // Fallback path: user typed an exact description but didn't arrow-highlight
  // it before pressing Enter. The autocomplete IIFE only consumes Enter when
  // there's a highlighted item, so we get this Enter unswallowed and try to
  // resolve typed text against the rendered options.
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Enter') return;
    var t = e.target;
    if (!(t instanceof HTMLInputElement)) return;
    if (!t.classList.contains('dep-input')) return;
    var picker = pickerOf(t);
    if (!picker) return;
    e.preventDefault(); // never let Enter submit the form from this input
    var typed = t.value.trim();
    if (typed === '') return;
    var resolved = resolveTyped(picker, typed);
    if (!resolved || !resolved.uuid) return;
    addPill(picker, resolved.uuid, resolved.label);
    t.value = '';
  });
  document.addEventListener('click', function (e) {
    var btn = e.target.closest && e.target.closest('.dep-pill-remove');
    if (!btn) return;
    var picker = pickerOf(btn);
    if (!picker) return;
    var pill = btn.closest('.dep-pill');
    if (pill && pill.parentNode) pill.parentNode.removeChild(pill);
    syncHidden(picker);
  });
})();
