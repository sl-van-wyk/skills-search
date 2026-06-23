// Package ui implements the terminal UI for browsing discovered skills.
package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sl-van-wyk/skills-search/finder"
)

// chrome reserves rows around the scrollable list: banner, divider, searchbox, gaps, and status.
var chrome = len(bannerArt) + 7

var bannerArt = []string{
	"███████╗██╗  ██╗██╗██╗     ██╗     ███████╗    ███████╗███████╗ █████╗ ██████╗  ██████╗██╗  ██╗",
	"██╔════╝██║ ██╔╝██║██║     ██║     ██╔════╝    ██╔════╝██╔════╝██╔══██╗██╔══██╗██╔════╝██║  ██║",
	"███████╗█████╔╝ ██║██║     ██║     ███████╗    ███████╗█████╗  ███████║██████╔╝██║     ███████║",
	"╚════██║██╔═██╗ ██║██║     ██║     ╚════██║    ╚════██║██╔══╝  ██╔══██║██╔══██╗██║     ██╔══██║",
	"███████║██║  ██╗██║███████╗███████╗███████║    ███████║███████╗██║  ██║██║  ██║╚██████╗██║  ██║",
	"╚══════╝╚═╝  ╚═╝╚═╝╚══════╝╚══════╝╚══════╝    ╚══════╝╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝",
}

// fallback dimensions before the first WindowSizeMsg arrives.
const defaultWidth = 80
const defaultHeight = 24

var (
	nameStyle   = lipgloss.NewStyle()
	sourceStyle = lipgloss.NewStyle().Faint(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	filterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	hintStyle   = lipgloss.NewStyle().Faint(true)
	emptyStyle  = lipgloss.NewStyle().Faint(true)
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	bannerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#c6d0f5"))
)

type mode int

const (
	modeNormal mode = iota
	modeConfirmDelete
	modeChooseCopyDest
)

// SkillEntry is a single skill with its display path and real filesystem path.
type SkillEntry struct {
	Name        string
	ShortSource string // tilde-shortened, for display
	FullSource  string // unexpanded real path, for filesystem operations
}

// foundMsg carries the result of the asynchronous skill walk.
type foundMsg struct {
	entries []SkillEntry
}

// refreshedMsg is a silent background re-walk result; does not affect loading state.
type refreshedMsg struct {
	entries []SkillEntry
}

// Model is the bubbletea model for the skills browser.
type Model struct {
	find     func() []SkillEntry
	entries  []SkillEntry
	selected map[int]bool // indices into m.entries; stable across filter changes
	filter   string
	cursor   int
	offset   int // first visible row index (viewport scroll position)
	width    int
	height   int
	loading  bool
	quitting bool
	mode     mode
	cwd      string // working directory at startup, used as copy-destination root
}

// New returns a Model. home tilde-shortens source paths; cwd is the copy-destination root.
func New(find func() []finder.SkillGroup, home, cwd string) Model {
	return Model{
		find: func() []SkillEntry {
			return flattenGroups(find(), home)
		},
		selected: make(map[int]bool),
		loading:  true,
		width:    defaultWidth,
		height:   defaultHeight,
		cwd:      cwd,
	}
}

// flattenGroups flattens SkillGroups into a flat list sorted by name then source.
func flattenGroups(groups []finder.SkillGroup, home string) []SkillEntry {
	var entries []SkillEntry
	for _, g := range groups {
		short := strings.Replace(g.Source, home, "~", 1)
		for _, s := range g.Skills {
			entries = append(entries, SkillEntry{Name: s, ShortSource: short, FullSource: g.Source})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].ShortSource < entries[j].ShortSource
	})
	return entries
}

// renderBanner returns the ASCII art header and full-width divider.
func (m Model) renderBanner() string {
	var b strings.Builder
	innerWidth := m.width - 1 // avoid terminal auto-wrap in the last column
	if innerWidth < 1 {
		innerWidth = 1
	}
	for _, line := range bannerArt {
		line = truncateToWidth(line, innerWidth)
		pad := (innerWidth - lipgloss.Width(line)) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(bannerStyle.Render(strings.Repeat(" ", pad) + line))
		b.WriteString("\n")
	}
	b.WriteString(bannerStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")
	return b.String()
}

func truncateToWidth(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	var b strings.Builder
	width := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if width+rw > maxWidth {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	return b.String()
}

// renderSearchBox returns a bordered input field with a placeholder when empty.
func (m Model) renderSearchBox() string {
	var input string
	if m.filter == "" {
		input = filterStyle.Render("> ") + hintStyle.Render("type to filter…")
	} else {
		input = filterStyle.Render("> " + m.filter)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Width(m.width - 4).
		Render(input)
}

// listHeight is the number of rows available for skill entries.
func (m Model) listHeight() int {
	h := m.height - chrome
	if h < 1 {
		return 1
	}
	return h
}

// Init kicks off the asynchronous skill walk.
func (m Model) Init() tea.Cmd {
	find := m.find
	return func() tea.Msg {
		return foundMsg{entries: find()}
	}
}

// refreshCmd fires a background re-walk without touching loading state.
func (m Model) refreshCmd() tea.Cmd {
	find := m.find
	return func() tea.Msg { return refreshedMsg{entries: find()} }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case foundMsg:
		m.entries = msg.entries
		m.loading = false
		m.clampCursor()
		m.clampOffset()
		return m, nil

	case refreshedMsg:
		type identity struct{ name, fullSource string }
		kept := make(map[identity]bool, len(m.selected))
		for idx := range m.selected {
			if idx < len(m.entries) {
				e := m.entries[idx]
				kept[identity{e.Name, e.FullSource}] = true
			}
		}
		m.entries = msg.entries
		newSelected := make(map[int]bool)
		for i, e := range m.entries {
			if kept[identity{e.Name, e.FullSource}] {
				newSelected[i] = true
			}
		}
		m.selected = newSelected
		m.clampCursor()
		m.clampOffset()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampOffset()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeConfirmDelete:
		if msg.Type == tea.KeyRunes && string(msg.Runes) == "D" {
			toDelete := make(map[int]bool)
			for idx := range m.selected {
				if idx < len(m.entries) {
					e := m.entries[idx]
					os.RemoveAll(filepath.Join(e.FullSource, e.Name)) //nolint:errcheck
					toDelete[idx] = true
				}
			}
			var remaining []SkillEntry
			for i, e := range m.entries {
				if !toDelete[i] {
					remaining = append(remaining, e)
				}
			}
			m.entries = remaining
			m.selected = make(map[int]bool)
			m.mode = modeNormal
			m.clampCursor()
			m.clampOffset()
			return m, m.refreshCmd()
		}
		m.mode = modeNormal
		return m, nil

	case modeChooseCopyDest:
		if msg.Type == tea.KeyRunes {
			switch string(msg.Runes) {
			case "1":
				m.executeCopy([]string{filepath.Join(m.cwd, ".agents", "skills")})
				m.mode = modeNormal
				return m, m.refreshCmd()
			case "2":
				m.executeCopy([]string{filepath.Join(m.cwd, ".claude", "skills")})
				m.mode = modeNormal
				return m, m.refreshCmd()
			case "3":
				m.executeCopy([]string{
					filepath.Join(m.cwd, ".agents", "skills"),
					filepath.Join(m.cwd, ".claude", "skills"),
				})
				m.mode = modeNormal
				return m, m.refreshCmd()
			}
		}
		m.mode = modeNormal
		return m, nil
	}

	// modeNormal
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit

	case tea.KeyEsc:
		if m.filter != "" {
			m.filter = ""
			m.clampCursor()
			m.clampOffset()
			return m, nil
		}
		m.quitting = true
		return m, tea.Quit

	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
		}
		m.clampOffset()
		return m, nil

	case tea.KeyDown:
		if m.cursor < m.selectableCount()-1 {
			m.cursor++
		}
		m.clampOffset()
		return m, nil

	case tea.KeySpace:
		visible := m.visibleEntries()
		if len(visible) > 0 && m.cursor < len(visible) {
			idx := m.entryIndex(visible[m.cursor])
			if idx >= 0 {
				if m.selected[idx] {
					delete(m.selected, idx)
				} else {
					m.selected[idx] = true
				}
			}
		}
		return m, nil

	case tea.KeyBackspace:
		if m.filter != "" {
			m.filter = m.filter[:len(m.filter)-1]
			m.clampCursor()
			m.clampOffset()
		}
		return m, nil

	case tea.KeyRunes:
		r := string(msg.Runes)
		// D and C are action shortcuts only when filter is empty (otherwise typed into filter).
		if m.filter == "" {
			switch r {
			case "q":
				m.quitting = true
				return m, tea.Quit
			case "D":
				if len(m.selected) > 0 {
					m.mode = modeConfirmDelete
				}
				return m, nil
			case "C":
				m.mode = modeChooseCopyDest
				return m, nil
			}
		}
		m.filter += r
		m.clampCursor()
		m.clampOffset()
		return m, nil
	}
	return m, nil
}

// executeCopy copies selected entries (or the cursor row) to each destination root.
func (m Model) executeCopy(destinations []string) {
	var targets []SkillEntry
	if len(m.selected) > 0 {
		for idx := range m.selected {
			if idx < len(m.entries) {
				targets = append(targets, m.entries[idx])
			}
		}
	} else {
		visible := m.visibleEntries()
		if len(visible) > 0 && m.cursor < len(visible) {
			targets = []SkillEntry{visible[m.cursor]}
		}
	}
	for _, e := range targets {
		src := filepath.Join(e.FullSource, e.Name)
		for _, dest := range destinations {
			copySkillDir(src, filepath.Join(dest, e.Name)) //nolint:errcheck
		}
	}
}

// copySkillDir copies src into dst; no-ops if dst already exists.
func copySkillDir(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	return os.CopyFS(dst, os.DirFS(src))
}

// View renders the current state.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.loading {
		return m.renderBanner() + emptyStyle.Render("Searching for skills…")
	}

	var inner strings.Builder

	inner.WriteString(m.renderBanner())
	inner.WriteString(m.renderSearchBox())
	inner.WriteString("\n")

	visible := m.visibleEntries()
	lh := m.listHeight()
	end := m.offset + lh
	if end > len(visible) {
		end = len(visible)
	}
	page := visible[m.offset:end]

	if len(visible) == 0 {
		inner.WriteString(emptyStyle.Render("No skills found"))
		inner.WriteString("\n")
	} else {
		contentWidth := m.width
		for i, e := range page {
			absIdx := m.offset + i
			fullIdx := m.entryIndex(e)
			check := "[ ]"
			if fullIdx >= 0 && m.selected[fullIdx] {
				check = "[x]"
			}
			// Truncate source so the entry always fits on one line.
			// namePartLen = "  " + check + " " + name (check is always 3 chars)
			namePartLen := 6 + len(e.Name)
			srcMaxWidth := contentWidth - namePartLen - 2 // -2 for "  " separator
			src := e.ShortSource
			if srcMaxWidth < 3 {
				src = ""
			} else if len(src) > srcMaxWidth {
				src = "…" + src[len(src)-(srcMaxWidth-1):]
			}
			if absIdx == m.cursor {
				inner.WriteString(cursorStyle.Render("▶ "+check+" "+e.Name) + "  " + sourceStyle.Render(src))
			} else {
				inner.WriteString(nameStyle.Render("  "+check+" "+e.Name) + "  " + sourceStyle.Render(src))
			}
			inner.WriteString("\n")
		}
	}

	inner.WriteString("\n")

	var status string
	selCount := len(m.selected)
	switch m.mode {
	case modeConfirmDelete:
		status = warnStyle.Render(fmt.Sprintf("⚠ Delete %d skill(s)? D to confirm · esc cancel", selCount))
	case modeChooseCopyDest:
		status = hintStyle.Render("Copy to: 1 .agents  2 .claude  3 both · esc cancel")
	default:
		if len(visible) == 0 {
			status = hintStyle.Render("0 results  ·  esc clear  ·  q quit")
		} else if selCount > 0 {
			status = hintStyle.Render(fmt.Sprintf("%d selected  ·  %d/%d  ·  ↑↓ navigate  ·  space toggle  ·  D delete  ·  C copy  ·  q quit",
				selCount, m.cursor+1, len(visible)))
		} else {
			status = hintStyle.Render(fmt.Sprintf("%d/%d  ·  ↑↓ navigate  ·  space toggle  ·  type to filter  ·  esc clear  ·  q quit",
				m.cursor+1, len(visible)))
		}
	}
	inner.WriteString(status)

	return inner.String()
}

// visibleEntries returns entries matching the filter (case-insensitive, name only).
func (m Model) visibleEntries() []SkillEntry {
	if m.filter == "" {
		return m.entries
	}
	needle := strings.ToLower(m.filter)
	var out []SkillEntry
	for _, e := range m.entries {
		if strings.Contains(strings.ToLower(e.Name), needle) {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) selectableCount() int {
	return len(m.visibleEntries())
}

func (m *Model) clampCursor() {
	max := m.selectableCount()
	if max == 0 {
		m.cursor = 0
		return
	}
	if m.cursor >= max {
		m.cursor = max - 1
	}
}

// entryIndex finds e in m.entries by Name+ShortSource; returns -1 if absent.
func (m Model) entryIndex(e SkillEntry) int {
	for i, entry := range m.entries {
		if entry.Name == e.Name && entry.ShortSource == e.ShortSource {
			return i
		}
	}
	return -1
}

// clampOffset keeps the cursor visible and prevents blank rows at the top.
func (m *Model) clampOffset() {
	lh := m.listHeight()
	total := m.selectableCount()

	if m.cursor >= m.offset+lh {
		m.offset = m.cursor - lh + 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	maxOffset := total - lh
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}
