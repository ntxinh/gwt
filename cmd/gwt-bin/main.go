package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

type listWorktreesMsg struct {
	wts []Worktree
	err error
}

type actionMsg struct{ err error }

type model struct {
	mode      mode
	wts       []Worktree
	cursor    int
	offset    int // first visible row index for scroll
	w, h      int // terminal dims from WindowSizeMsg
	inputs    [2]textinput.Model
	focus     int
	selected  int // worktree index while confirming delete
	status    string
	adding    bool // true while an Add actionMsg is in flight
	currentWt int  // index of worktree containing cwd, -1 when none
}

func initialModel() model {
	dir := textinput.New()
	dir.Placeholder = "directory name"
	dir.Focus()
	branch := textinput.New()
	branch.Placeholder = "branch (empty = auto-name)"

	return model{mode: modeList, selected: -1, currentWt: -1, inputs: [2]textinput.Model{dir, branch}}
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
		m.w, m.h = msg.Width, msg.Height
		return m, nil
	case listWorktreesMsg:
		if msg.err != nil {
			m.status = msg.err.Error()
			return m, nil
		}
		m.wts = msg.wts
		m.currentWt = currentWorktree(msg.wts)
		if m.cursor >= len(m.wts) {
			m.cursor = max(0, len(m.wts)-1)
		}
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

// currentWorktree finds the worktree whose path contains cwd (lazygit marks it *).
func currentWorktree(wts []Worktree) int {
	cwd, err := os.Getwd()
	if err != nil {
		return -1
	}
	best, bestLen := -1, -1
	for i, w := range wts {
		if strings.HasPrefix(cwd+string(os.PathSeparator), w.Path+string(os.PathSeparator)) && len(w.Path) > bestLen {
			best, bestLen = i, len(w.Path)
		}
	}
	return best
}

// moveCursor adjusts the cursor and keeps it inside the visible window.
func (m *model) moveCursor(d int) {
	m.cursor = clamp(m.cursor+d, 0, max(0, len(m.wts)-1))
	vis := m.listVisibleRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	} else if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
	}
}

func (m model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "q", "esc":
			return m, tea.Quit
		case "enter":
			if len(m.wts) > 0 {
				fmt.Println(m.wts[m.cursor].Path) // the only stdout write: jump target
			}
			return m, tea.Quit
		case "j", "down":
			m.moveCursor(1)
		case "k", "up":
			m.moveCursor(-1)
		case "g":
			m.moveCursor(-len(m.wts))
		case "G":
			m.moveCursor(len(m.wts))
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
			if len(m.wts) > 0 {
				m.selected = m.cursor
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
	return m, nil
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
	if len(m.wts) == 0 {
		m.status = "no worktree list loaded"
		return m, nil
	}
	root := m.wts[0].Path // first porcelain record = main worktree
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
			path := m.wts[m.selected].Path
			m.mode = modeList
			m.selected = -1
			return m, doCmd(func() error { return removeWorktree(path) })
		case "n", "N", "esc", "enter":
			m.mode = modeList
			m.selected = -1
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

func main() {
	// UI renders on stderr; stdout is piped through the fish wrapper, so a
	// stdout-profiled renderer would see a pipe and emit no colors.
	lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(os.Stderr))
	initStyles()                               // styles capture the renderer at creation — must follow the bind
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
