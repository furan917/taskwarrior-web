// context_picker.js - the Add modal's context picker dropdown. On change,
// silently overwrite Tags/Project inputs with the selected option's
// data-prefill values, and update the helper text below.
// ─── Add-modal context picker (v5) ──────────────────────────────────────
// The Add Task modal carries a context-picker dropdown in its header. On
// change, we silently overwrite the Tags + Project inputs with the selected
// option's data-prefill-tags / data-prefill-project, and update the helper
// text underneath. This is purely a UX shortcut - the form still submits
// raw Tags/Project values; "context" is not a Taskwarrior task field.
//
// Helper-text format MUST match views.ContextHelperText so the initial
// server-rendered text and our JS-rendered text don't visibly jump on
// first user interaction. Both encode the same prose contract:
//   tag empty + project empty -> "No context tag/project will be added."
//   tag set                   -> "Adds +<tag> tag"
//   project set               -> "Sets Project = <project>"
//   both                      -> "<tag-clause>, <project-clause>."
(function () {
  function helperTextFor(tag, project) {
    if (tag === '' && project === '') {
      return 'No context tag/project will be added.';
    }
    var parts = [];
    if (tag !== '') parts.push('Adds +' + tag + ' tag');
    if (project !== '') parts.push('Sets Project = ' + project);
    return parts.join(', ') + '.';
  }

  document.addEventListener('change', function (e) {
    var sel = e.target.closest && e.target.closest('[data-context-select]');
    if (!sel) return;
    var dialog = sel.closest('dialog');
    if (!dialog) return;
    var opt = sel.options[sel.selectedIndex];
    if (!opt) return;
    var tag = opt.getAttribute('data-prefill-tags') || '';
    var project = opt.getAttribute('data-prefill-project') || '';

    // Silent overwrite of Tags + Project. Per design (silent addition):
    // any custom values the user typed before changing the context get
    // replaced - they can re-edit afterwards. The helper text below the
    // dropdown is the always-visible "what will be added" reassurance.
    var projectInput = dialog.querySelector('input[name="project"]');
    var tagsInput = dialog.querySelector('input[name="tags"]');
    if (projectInput) projectInput.value = project;
    if (tagsInput) tagsInput.value = tag;

    var helper = dialog.querySelector('[data-context-helper]');
    if (helper) helper.textContent = helperTextFor(tag, project);
  });
})();
