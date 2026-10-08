package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type mode int

const (
	modeList mode = iota
	modeAdd
	modeConfirmDelete
	modeConfirmPrune
)

type item struct{ wt Worktree }

func (i item) Title() string {
	t := i.wt.Path // spec §6: row shows the path; locked gets a * suffix
	if i.wt.Locked {
		t += " *"
	}
	return t
}
func (i item) Description() string { return i.wt.BranchName() + "  " + i.wt.ShortHead() }
func (i item) FilterValue() string { return i.wt.Path }

type listWorktreesMsg struct {
	wts []Worktree
	err error
}

type actionMsg struct{ err error }

type model struct {
	mode     mode
	list     list.Model
	inputs   [2]textinput.Model
	focus    int
	selected *Worktree
	status   string
	adding   bool // true while an Add actionMsg is in flight
}

var (
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

func initialModel() model {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "git worktrees"
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetShowFilter(false)
	l.SetFilteringEnabled(false)
	l.SetShowPagination(false)

	dir := textinput.New()
	dir.Placeholder = "directory name"
	dir.Focus()
	branch := textinput.New()
	branch.Placeholder = "branch (empty = auto-name)"

	return model{mode: modeList, list: l, inputs: [2]textinput.Model{dir, branch}}
}

func listWorktreesCmd() tea.Msg {
	wts, err := listWorktrees()
	return listWorktreesMsg{wts: wts, err: err}
}

func doCmd(fn func() error) tea.Cmd {
	return func() tea.Msg { return actionMsg{err: fn()} }
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, listWorktreesCmd)
}

// Update routes messages that matter in every mode first, then dispatches
// by mode. Key handling per spec state machine.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-2)
		return m, nil
	case listWorktreesMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		items := make([]list.Item, len(msg.wts))
		for i, w := range msg.wts {
			items[i] = item{wt: w}
		}
		m.list.SetItems(items)
		// NOTE: do NOT clear m.status here — the reload that follows every
		// mutation must not erase a just-set error message.
		return m, nil
	case actionMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			if m.adding && m.mode == modeList { // failed add → back to form only if user is still there
				m.mode = modeAdd
			}
		} else {
			m.status = ""
		}
		m.adding = false
		return m, listWorktreesCmd // reload after every mutation
	}

	k, isKey := msg.(tea.KeyMsg)
	if isKey && k.String() == "ctrl+c" {
		return m, tea.Quit
	}

	switch m.mode {
	case modeList:
		return m.updateList(msg)
	case modeAdd:
		return m.updateAdd(msg)
	case modeConfirmDelete:
		return m.updateConfirmDelete(msg)
	case modeConfirmPrune:
		return m.updateConfirmPrune(msg)
	}
	return m, nil
}

func (m model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "esc":
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(item); ok {
				fmt.Println(it.wt.Path) // the only stdout write: jump target
			}
			return m, tea.Quit
		case "a":
			m.mode = modeAdd
			m.status = ""
			m.focus = 0
			m.inputs[0].SetValue("")
			m.inputs[1].SetValue("")
			m.inputs[0].Focus()
			m.inputs[1].Blur()
			return m, textinput.Blink
		case "d":
			if it, ok := m.list.SelectedItem().(item); ok {
				m.selected = &it.wt
				m.mode = modeConfirmDelete
			}
			return m, nil
		case "p":
			m.mode = modeConfirmPrune
			return m, nil
		case "r":
			return m, listWorktreesCmd
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg) // j/k, arrows, g/G via list
	return m, cmd
}

func (m model) updateAdd(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.mode = modeList
			m.status = ""
			return m, nil
		case "tab", "shift+tab", "up", "down":
			m.focus = (m.focus + 1) % len(m.inputs)
			for i := range m.inputs {
				if i == m.focus {
					m.inputs[i].Focus()
				} else {
					m.inputs[i].Blur()
				}
			}
			return m, textinput.Blink
		case "enter":
			return m.submitAdd()
		}
	}
	var cmds []tea.Cmd
	for i := range m.inputs {
		var c tea.Cmd
		m.inputs[i], c = m.inputs[i].Update(msg)
		cmds = append(cmds, c)
	}
	return m, tea.Batch(cmds...)
}

func (m model) submitAdd() (tea.Model, tea.Cmd) {
	dirName := strings.TrimSpace(m.inputs[0].Value())
	branch := strings.TrimSpace(m.inputs[1].Value())
	if dirName == "" || strings.ContainsAny(dirName, `/\`) {
		m.status = "directory name required (no slashes)"
		return m, nil
	}
	items := m.list.Items()
	if len(items) == 0 {
		m.status = "no worktree list loaded"
		return m, nil
	}
	root := items[0].(item).wt.Path // first porcelain record = main worktree
	path := filepath.Join(root, ".worktrees", dirName)
	m.status = ""
	m.mode = modeList // spec §6: success → List; on failure actionMsg returns to modeAdd
	m.adding = true
	return m, doCmd(func() error {
		if err := ensureExcluded(); err != nil {
			return err
		}
		return addWorktree(path, branch)
	})
}

func (m model) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y":
			path := m.selected.Path
			m.mode = modeList
			m.selected = nil
			return m, doCmd(func() error { return removeWorktree(path) })
		case "n", "N", "esc", "enter":
			m.mode = modeList
			m.selected = nil
		}
	}
	return m, nil
}

func (m model) updateConfirmPrune(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "y", "Y":
			m.mode = modeList
			return m, doCmd(pruneWorktrees)
		case "n", "N", "esc", "enter":
			m.mode = modeList
		}
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	switch m.mode {
	case modeList:
		b.WriteString(m.list.View())
	case modeAdd:
		b.WriteString("New worktree\n\n")
		b.WriteString("dir:    " + m.inputs[0].View() + "\n")
		b.WriteString("branch: " + m.inputs[1].View() + "\n\n")
		b.WriteString(dimStyle.Render("tab: switch field • enter: create • esc: cancel"))
	case modeConfirmDelete:
		name := filepath.Base(m.selected.Path)
		if bn := m.selected.BranchName(); bn != "(detached)" && bn != "(bare)" {
			name = bn
		}
		fmt.Fprintf(&b, "Force remove worktree %s? (y/N)", name)
	case modeConfirmPrune:
		b.WriteString("Prune missing/orphaned worktrees? (y/N)")
	}
	if m.status != "" {
		b.WriteString("\n" + statusStyle.Render(m.status))
	}
	return b.String()
}

func main() {
	if _, err := listWorktrees(); err != nil { // spec §4 fatal path: stderr + exit 1
		fmt.Fprintln(os.Stderr, "gwt:", err)
		os.Exit(1)
	}
	p := tea.NewProgram(initialModel(), tea.WithOutput(os.Stderr))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gwt:", err)
		os.Exit(1)
	}
}
