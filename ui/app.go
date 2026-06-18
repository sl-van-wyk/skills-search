// Package ui implements the terminal UI for browsing discovered skills.
package ui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sl-van-wyk/skills-search/finder"
)

// chrome is the number of rows consumed by non-list elements:
//
//	border top (1) + filter row (1) + spacer (1) + separator (1) + status row (1) + border bottom (1)
const chrome = 6

// default terminal dimensions used before the first WindowSizeMsg arrives.
const defaultWidth = 80
const defaultHeight = 24

var (
	nameStyle   = lipgloss.NewStyle()
	sourceStyle = lipgloss.NewStyle().Faint(true)
	cursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
	filterStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	hintStyle   = lipgloss.NewStyle().Faint(true)
	emptyStyle  = lipgloss.NewStyle().Faint(true)
)

// SkillEntry is a single skill with its tilde-shortened source path.
type SkillEntry struct {
	Name        string
	ShortSource string
}

// foundMsg carries the result of the asynchronous skill walk.
type foundMsg struct {
	entries []SkillEntry
}

// Model is the bubbletea model for the skills browser.
type Model struct {
	find     func() []SkillEntry
	entries  []SkillEntry
	selected map[int]bool // keys are indices into m.entries (persists through filter changes)
	filter   string
	cursor   int
	offset   int // first visible row index (viewport scroll position)
	width    int
	height   int
	loading  bool
	quitting bool
}

// New returns a Model. find is called once asynchronously after the program
// starts. home is used to tilde-shorten source paths.
func New(find func() []finder.SkillGroup, home string) Model {
	return Model{
		find: func() []SkillEntry {
			return flattenGroups(find(), home)
		},
		selected: make(map[int]bool),
		loading:  true,
		width:    defaultWidth,
		height:   defaultHeight,
	}
}

// flattenGroups converts grouped skill results into a flat sorted list.
// Sorted by name then source for stable output.
func flattenGroups(groups []finder.SkillGroup, home string) []SkillEntry {
	var entries []SkillEntry
	for _, g := range groups {
		short := strings.Replace(g.Source, home, "~", 1)
		for _, s := range g.Skills {
			entries = append(entries, SkillEntry{Name: s, ShortSource: short})
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

// Update handles incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case foundMsg:
		m.entries = msg.entries
		m.loading = false
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
		runes := string(msg.Runes)
		if runes == "q" && m.filter == "" {
			m.quitting = true
			return m, tea.Quit
		}
		m.filter += runes
		m.clampCursor()
		m.clampOffset()
		return m, nil
	}
	return m, nil
}

// View renders the current state within a rounded border box.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.loading {
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Width(m.width - 2).
			Render(emptyStyle.Render("Searching for skills…"))
	}

	var inner strings.Builder

	// Filter line.
	inner.WriteString(filterStyle.Render("> " + m.filter))
	inner.WriteString("\n\n")

	// Skill rows — sliced to the viewport window.
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
		for i, e := range page {
			absIdx := m.offset + i
			fullIdx := m.entryIndex(e)
			check := "[ ]"
			if fullIdx >= 0 && m.selected[fullIdx] {
				check = "[x]"
			}
			if absIdx == m.cursor {
				inner.WriteString(cursorStyle.Render("▶ "+check+" "+e.Name) + "  " + sourceStyle.Render(e.ShortSource))
			} else {
				inner.WriteString(nameStyle.Render("  "+check+" "+e.Name) + "  " + sourceStyle.Render(e.ShortSource))
			}
			inner.WriteString("\n")
		}
	}

	// Separator before status.
	inner.WriteString("\n")

	// Status line.
	var status string
	selCount := len(m.selected)
	if len(visible) == 0 {
		status = "0 results  ·  esc clear  ·  q quit"
	} else if selCount > 0 {
		status = fmt.Sprintf("%d selected  ·  %d/%d  ·  ↑↓ navigate  ·  space toggle  ·  type to filter  ·  q quit",
			selCount, m.cursor+1, len(visible))
	} else {
		status = fmt.Sprintf("%d/%d  ·  ↑↓ navigate  ·  space toggle  ·  type to filter  ·  esc clear  ·  q quit",
			m.cursor+1, len(visible))
	}
	inner.WriteString(hintStyle.Render(status))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(m.width - 2).
		Render(inner.String())
}

// visibleEntries returns entries filtered by the current substring filter
// (case-insensitive on name only).
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

// selectableCount is the total number of currently visible entries.
func (m Model) selectableCount() int {
	return len(m.visibleEntries())
}

// clampCursor keeps the cursor within [0, selectableCount-1].
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

// entryIndex returns the index of e in m.entries by matching Name+ShortSource.
// Returns -1 if not found.
func (m Model) entryIndex(e SkillEntry) int {
	for i, entry := range m.entries {
		if entry.Name == e.Name && entry.ShortSource == e.ShortSource {
			return i
		}
	}
	return -1
}

// clampOffset adjusts the viewport scroll offset to keep the cursor visible
// and ensures no trailing blank rows when the list is shorter than the viewport.
func (m *Model) clampOffset() {
	lh := m.listHeight()
	total := m.selectableCount()

	// Scroll down: cursor has moved below the visible window.
	if m.cursor >= m.offset+lh {
		m.offset = m.cursor - lh + 1
	}
	// Scroll up: cursor has moved above the visible window.
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	// Clamp so we never show blank rows at the top when the list shrinks.
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
