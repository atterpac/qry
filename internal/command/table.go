package command

import "strings"

// Command is a single command-bar entry. Run is a closure bound to whatever
// host (e.g. the view app) registered it, so the command package stays free of
// view-layer dependencies.
type Command struct {
	Name    string
	Aliases []string
	Usage   string
	Run     func(args []string)
}

// Table is an ordered registry of commands with alias lookup. The order is
// preserved for help/autocomplete listings.
type Table struct {
	commands []*Command
	index    map[string]*Command
}

// NewTable returns an empty command table.
func NewTable() *Table {
	return &Table{index: make(map[string]*Command)}
}

// Add registers a command under its name and any aliases. It returns the table
// for chaining. Duplicate names/aliases overwrite the previous entry in the
// index but the command list keeps insertion order.
func (t *Table) Add(c *Command) *Table {
	t.commands = append(t.commands, c)
	t.index[c.Name] = c
	for _, a := range c.Aliases {
		t.index[a] = c
	}
	return t
}

// Lookup resolves a command by name or alias.
func (t *Table) Lookup(name string) (*Command, bool) {
	c, ok := t.index[name]
	return c, ok
}

// Commands returns the registered commands in insertion order.
func (t *Table) Commands() []*Command {
	return t.commands
}

// Dispatch parses text into a command name and arguments and resolves it.
// It does not invoke the command — the caller decides what to do when found is
// false (e.g. show an "unknown command" warning). name is the first whitespace-
// delimited token (empty when text is blank).
func (t *Table) Dispatch(text string) (cmd *Command, name string, args []string, found bool) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) == 0 {
		return nil, "", nil, false
	}
	name = parts[0]
	args = parts[1:]
	cmd, found = t.Lookup(name)
	return cmd, name, args, found
}
