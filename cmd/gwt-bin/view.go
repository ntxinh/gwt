package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// lazygit theme: rounded borders, green+bold active border, blue options bar,
// red for problems, green * for the current worktree.
var (
	activeBorder   = lipgloss.Color("10") // green bold
	inactiveBorder = lipgloss.Color("8")
	selStyle       lipgloss.Style
	curStyle       lipgloss.Style
	warnStyle      lipgloss.Style // red: errors, prunable
	lockStyle      lipgloss.Style // yellow: locked, detached
	branchStyle    lipgloss.Style // cyan: branch names (lazygit FgCyan)
	dimStyle       lipgloss.Style
	optsStyle      lipgloss.Style // blue: options bar (lazygit OptionsTextColor)
	modalStyle     lipgloss.Style
)

// initStyles builds the styles AFTER main has bound lipgloss's default
// renderer to stderr — package-level NewStyle() would capture the
// stdout-based renderer at init and see a pipe under `gwt()`, never a TTY.
func initStyles() {
	selStyle = lipgloss.NewStyle().Bold(true).Background(lipgloss.Color("236"))
	curStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	lockStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	branchStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("14"))
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	optsStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	modalStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(activeBorder).Padding(0, 1)
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// panel wraps content in a rounded border with a title in the top-left corner.
// w, h are the panel's total (outer) dimensions.
func panel(title, content string, w, h int, active bool) string {
	borderCol := inactiveBorder
	if active {
		borderCol = activeBorder
	}
	bs := lipgloss.NewStyle().Foreground(borderCol)
	t := " " + title + " "
	top := bs.Render("╭─" + ansi.Truncate(t, w-4, "") + strings.Repeat("─", max(0, w-4-lipgloss.Width(t))) + "╮")
	body := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), false, true, true, true). // no top: title row is hand-built
		BorderForeground(borderCol).
		Width(w - 2).
		Height(h - 2).
		Render(content)
	return top + "\n" + body
}

func (m model) panelHeight() int { return max(4, m.h-1) } // 1 row for options bar
func (m model) listVisibleRows() int {
	return max(1, m.panelHeight()-2)
}

func (m model) rowText(wt Worktree, i int, nameW int) string {
	name := filepath.Base(wt.Path)
	var mark, flags string
	if i == m.currentWt {
		mark = curStyle.Render("*") + " "
	}
	// lazygit palette: branch cyan; detached HEAD yellow "(detached at <sha>)"
	branch := branchStyle.Render(wt.BranchName())
	if wt.Detached {
		branch = lockStyle.Render("(detached at " + wt.ShortHead() + ")")
	}
	var f []string
	if wt.Locked {
		f = append(f, lockStyle.Render("locked"))
	}
	if wt.Prunable {
		f = append(f, warnStyle.Render("prunable"))
	}
	if len(f) > 0 {
		flags = " [" + strings.Join(f, " ") + "]"
	}
	gap := max(0, nameW-lipgloss.Width(name)) + 2
	return mark + name + strings.Repeat(" ", gap) + branch + flags
}

func (m model) worktreeRows(width int) string {
	if len(m.wts) == 0 {
		return dimStyle.Render("no worktrees")
	}
	nameW := 0
	for _, w := range m.wts {
		nameW = max(nameW, lipgloss.Width(filepath.Base(w.Path)))
	}
	var b strings.Builder
	vis := m.listVisibleRows()
	for i := m.offset; i < len(m.wts) && i < m.offset+vis; i++ {
		row := ansi.Truncate(m.rowText(m.wts[i], i, nameW), width, "")
		if i == m.cursor {
			row = selStyle.Width(width).Render(row)
		}
		b.WriteString(row + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m model) detailsText() string {
	if len(m.wts) == 0 || m.cursor >= len(m.wts) {
		return ""
	}
	w := m.wts[m.cursor]
	var flags []string
	if w.Bare {
		flags = append(flags, lockStyle.Render("bare"))
	}
	if w.Detached {
		flags = append(flags, lockStyle.Render("detached"))
	}
	if w.Locked {
		flags = append(flags, lockStyle.Render("locked"))
	}
	if w.Prunable {
		flags = append(flags, warnStyle.Render("prunable"))
	}
	if len(flags) == 0 {
		flags = append(flags, dimStyle.Render("—"))
	}
	return fmt.Sprintf("%s %s\n\n%s %s\n%s %s\n%s %s",
		dimStyle.Render("Path  "), w.Path,
		dimStyle.Render("Branch"), branchStyle.Render(w.BranchName()),
		dimStyle.Render("HEAD  "), lockStyle.Render(w.ShortHead()),
		dimStyle.Render("Flags "), strings.Join(flags, ", "))
}

const optionsBar = "<enter> jump   <a> add   <d> delete   <p> prune   <r> reload   <q> quit"

func (m model) optionsLine() string {
	opts := optsStyle.Render(optionsBar)
	if m.status == "" {
		return opts
	}
	return warnStyle.Render(m.status) + "  " + opts
}

// layout renders the base screen: list + details panels over the options bar.
func (m model) layout() string {
	ph := m.panelHeight()
	lw := clamp(m.w/3, 24, 48)
	dw := max(10, m.w-lw)
	left := panel("Worktrees", m.worktreeRows(lw-2), lw, ph, true)
	right := panel("Details", m.detailsText(), dw, ph, false)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n" + m.optionsLine()
}

func (m model) View() string {
	base := m.layout()
	switch m.mode {
	case modeAdd:
		box := modalStyle.Render("New worktree\n\n" +
			"dir:    " + m.inputs[0].View() + "\n" +
			"branch: " + m.inputs[1].View() + "\n\n" +
			dimStyle.Render("tab: switch field • enter: create • esc: cancel"))
		return overlayBox(base, box, m.w)
	case modeConfirmDelete:
		name := filepath.Base(m.wts[m.selected].Path)
		if bn := m.wts[m.selected].BranchName(); bn != "(detached)" && bn != "(bare)" {
			name = bn
		}
		box := modalStyle.Render(fmt.Sprintf("Force remove worktree %s? (y/N)", name))
		return overlayBox(base, box, m.w)
	case modeConfirmPrune:
		box := modalStyle.Render("Prune missing/orphaned worktrees? (y/N)")
		return overlayBox(base, box, m.w)
	}
	return base
}

// overlayBox replaces the base's cells under the box, keeping surrounding
// borders visible like lazygit's centered popups. totalW = terminal width.
func overlayBox(base, box string, totalW int) string {
	bw, bh := lipgloss.Width(box), lipgloss.Height(box)
	baseLines := strings.Split(base, "\n")
	ox := (totalW - bw) / 2
	oy := (len(baseLines) - bh) / 2
	if ox < 0 {
		ox = 0
	}
	if oy < 0 {
		oy = 0
	}
	boxLines := strings.Split(box, "\n")
	for i, bl := range boxLines {
		row := oy + i
		if row >= len(baseLines) {
			break
		}
		baseLines[row] = ansi.Cut(baseLines[row], 0, ox) + bl +
			ansi.Cut(baseLines[row], ox+bw, totalW)
	}
	return strings.Join(baseLines, "\n")
}
