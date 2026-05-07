package views

// Keybinding is one entry in the help dialog and the footer hint. Both
// surfaces render from the same slice so a new binding only has to be added
// once and the two views stay in lockstep.
//
// FooterLabel is the short label used in the footer (e.g. "down"); HelpDesc
// is the longer description shown in the help dialog ("Focus next task").
// InFooter / RowListOnly gate which entries appear in the footer:
//
//   - InFooter=false: help dialog only (e.g. "*", "Esc")
//   - InFooter=true,  RowListOnly=true:  footer only when p.HasTaskList
//   - InFooter=true,  RowListOnly=false: footer always (e.g. "n", "?")
type Keybinding struct {
	Key         string
	FooterLabel string
	HelpDesc    string
	InFooter    bool
	RowListOnly bool
}

// Keybindings is the canonical list. layout.templ's helpDialog and keyHint
// both iterate this slice; the JS handlers in app.js implement the actual
// behaviour and are listed in the same order for cross-reference.
var Keybindings = []Keybinding{
	{Key: "n", FooterLabel: "add", HelpDesc: "New task", InFooter: true, RowListOnly: false},
	{Key: "j", FooterLabel: "down", HelpDesc: "Focus next task", InFooter: true, RowListOnly: true},
	{Key: "k", FooterLabel: "up", HelpDesc: "Focus previous task", InFooter: true, RowListOnly: true},
	{Key: "Enter", FooterLabel: "edit", HelpDesc: "Edit focused task", InFooter: true, RowListOnly: true},
	{Key: "x", FooterLabel: "done", HelpDesc: "Mark focused task done", InFooter: true, RowListOnly: true},
	{Key: "Space", FooterLabel: "select", HelpDesc: "Toggle focused row's checkbox", InFooter: true, RowListOnly: true},
	{Key: "*", HelpDesc: "Select all visible rows"},
	{Key: "Esc", HelpDesc: "Close dialog or clear selection"},
	{Key: "?", FooterLabel: "help", HelpDesc: "Show this help", InFooter: true, RowListOnly: false},
}
