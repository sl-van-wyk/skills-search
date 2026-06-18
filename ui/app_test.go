package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/sl-van-wyk/skills-search/finder"
)

const fakeHome = "/home/user"
const fakeCwd = "/tmp/fake-cwd"

func testGroups() []finder.SkillGroup {
	return []finder.SkillGroup{
		{Source: "/home/user/.claude/skills", Skills: []string{"databricks-core", "fastapi"}},
		{Source: "/home/user/.agents/skills", Skills: []string{"openspec-explore"}},
	}
}

// loaded returns a Model that has already received its walk results.
func loaded(groups func() []finder.SkillGroup) Model {
	m := New(groups, fakeHome, fakeCwd)
	next, _ := m.Update(foundMsg{entries: flattenGroups(groups(), fakeHome)})
	return next.(Model)
}

// runes builds a KeyRunes message for the given string.
func runes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestViewLoadingState(t *testing.T) {
	m := New(testGroups, fakeHome, fakeCwd)
	if !strings.Contains(m.View(), "Searching") {
		t.Errorf("loading view = %q, want mention of searching", m.View())
	}
}

func TestFlattenGroups(t *testing.T) {
	entries := flattenGroups(testGroups(), fakeHome)

	// Should be sorted alphabetically by name.
	wantOrder := []string{"databricks-core", "fastapi", "openspec-explore"}
	if len(entries) != len(wantOrder) {
		t.Fatalf("flattenGroups returned %d entries, want %d", len(entries), len(wantOrder))
	}
	for i, e := range entries {
		if e.Name != wantOrder[i] {
			t.Errorf("entries[%d].Name = %q, want %q", i, e.Name, wantOrder[i])
		}
	}

	// Source paths should be tilde-shortened.
	for _, e := range entries {
		if strings.Contains(e.ShortSource, fakeHome) {
			t.Errorf("ShortSource %q still contains home path", e.ShortSource)
		}
		if !strings.HasPrefix(e.ShortSource, "~") {
			t.Errorf("ShortSource %q should start with ~", e.ShortSource)
		}
	}

	// FullSource should be the unexpanded path.
	for _, e := range entries {
		if strings.HasPrefix(e.FullSource, "~") {
			t.Errorf("FullSource %q should not be tilde-shortened", e.FullSource)
		}
		if e.FullSource == "" {
			t.Errorf("FullSource should not be empty for entry %q", e.Name)
		}
	}
}

func TestFlattenGroupsDuplicateNames(t *testing.T) {
	groups := []finder.SkillGroup{
		{Source: "/home/user/.claude/skills", Skills: []string{"fastapi"}},
		{Source: "/home/user/.agents/skills", Skills: []string{"fastapi"}},
	}
	entries := flattenGroups(groups, fakeHome)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (no dedup), got %d", len(entries))
	}
	if entries[0].Name != "fastapi" || entries[1].Name != "fastapi" {
		t.Errorf("both entries should be 'fastapi': %v", entries)
	}
	// Different sources.
	if entries[0].ShortSource == entries[1].ShortSource {
		t.Error("duplicate skill entries should have different sources")
	}
}

func TestViewShowsFlatRows(t *testing.T) {
	out := loaded(testGroups).View()

	// All skill names visible.
	for _, want := range []string{"databricks-core", "fastapi", "openspec-explore"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q\n--- view ---\n%s", want, out)
		}
	}

	// Source paths tilde-shortened and present.
	for _, want := range []string{"~/.claude/skills", "~/.agents/skills"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing source %q\n--- view ---\n%s", want, out)
		}
	}

	// No group headers (raw source dir paths should not appear as standalone lines).
	if strings.Contains(out, "/home/user") {
		t.Errorf("view should not contain raw home path, got:\n%s", out)
	}
}

func TestViewDuplicateNamesShowTwice(t *testing.T) {
	groups := func() []finder.SkillGroup {
		return []finder.SkillGroup{
			{Source: "/home/user/.claude/skills", Skills: []string{"fastapi"}},
			{Source: "/home/user/.agents/skills", Skills: []string{"fastapi"}},
		}
	}
	out := loaded(groups).View()
	count := strings.Count(out, "fastapi")
	if count != 2 {
		t.Errorf("expected 'fastapi' to appear twice (non-deduped), got %d times\n%s", count, out)
	}
}

func TestFilterNarrowsByName(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(runes("fast"))
	out := next.(Model).View()

	if !strings.Contains(out, "fastapi") {
		t.Errorf("filtered view should show fastapi:\n%s", out)
	}
	if strings.Contains(out, "databricks-core") {
		t.Errorf("filtered view should hide databricks-core:\n%s", out)
	}
	if strings.Contains(out, "openspec-explore") {
		t.Errorf("filtered view should hide openspec-explore:\n%s", out)
	}
}

func TestFilterCaseInsensitive(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(runes("FAST"))
	if !strings.Contains(next.(Model).View(), "fastapi") {
		t.Error("uppercase filter should match lowercase skill name")
	}
}

func TestFilterDoesNotMatchSource(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(runes(".claude"))
	out := next.(Model).View()
	if !strings.Contains(out, "No skills found") {
		t.Errorf("filter on source path should find nothing:\n%s", out)
	}
}

func TestEmptyFilterShowsAll(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(runes("fast"))
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	out := next.(Model).View()

	for _, want := range []string{"databricks-core", "fastapi", "openspec-explore"} {
		if !strings.Contains(out, want) {
			t.Errorf("cleared filter should show %q again:\n%s", want, out)
		}
	}
}

func TestFilterNoMatchShowsMessage(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(runes("zzz-nope"))
	if !strings.Contains(next.(Model).View(), "No skills found") {
		t.Errorf("non-matching filter should show empty-state:\n%s", next.(Model).View())
	}
}

func TestBackspaceRemovesChar(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(runes("fastx"))
	next, _ = next.(Model).Update(tea.KeyMsg{Type: tea.KeyBackspace})
	out := next.(Model).View()

	if !strings.Contains(out, "fastapi") {
		t.Errorf("after backspace, 'fast' should match fastapi:\n%s", out)
	}
}

func TestEmptyResultsShowsMessage(t *testing.T) {
	empty := func() []finder.SkillGroup { return nil }
	if !strings.Contains(loaded(empty).View(), "No skills found") {
		t.Error("no results should show empty-state message")
	}
}

func TestNavigationClamped(t *testing.T) {
	m := loaded(testGroups) // 3 entries
	var model tea.Model = m
	for i := 0; i < 10; i++ {
		model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	if got := model.(Model).cursor; got != 2 {
		t.Errorf("cursor = %d, want clamped to 2", got)
	}
	for i := 0; i < 10; i++ {
		model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyUp})
	}
	if got := model.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d, want clamped to 0", got)
	}
}

func TestCursorReclampedOnFilter(t *testing.T) {
	m := loaded(testGroups)
	var model tea.Model = m
	for i := 0; i < 2; i++ {
		model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	model, _ = model.(Model).Update(runes("fast"))
	if got := model.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d, want reclamped to 0 after filter", got)
	}
}

// --- selection tests ---

func space() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeySpace} }

func TestSpacebarSelectsSkill(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(space())
	if !strings.Contains(next.(Model).View(), "[x]") {
		t.Errorf("pressing space should show [x] on the cursor row:\n%s", next.(Model).View())
	}
}

func TestSpacebarDeselectsSkill(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(space())
	next, _ = next.(Model).Update(space())
	out := next.(Model).View()
	if strings.Contains(out, "[x]") {
		t.Errorf("pressing space twice should deselect — no [x] expected:\n%s", out)
	}
}

func TestSelectionPersistsThroughFilter(t *testing.T) {
	m := loaded(testGroups) // entries sorted: databricks-core, fastapi, openspec-explore
	// Select fastapi (index 1 in full list — move cursor down once).
	var model tea.Model = m
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor on fastapi
	model, _ = model.(Model).Update(space())

	// Apply filter that keeps fastapi visible.
	model, _ = model.(Model).Update(runes("fast"))
	if !strings.Contains(model.(Model).View(), "[x]") {
		t.Errorf("selection should persist when filter keeps the skill visible:\n%s", model.(Model).View())
	}

	// Clear filter — still selected.
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(model.(Model).View(), "[x]") {
		t.Errorf("selection should persist after filter is cleared:\n%s", model.(Model).View())
	}
}

func TestSelectionSurvivesFilterThatHidesSkill(t *testing.T) {
	m := loaded(testGroups)
	// Select databricks-core (cursor starts there, index 0).
	var model tea.Model = m
	model, _ = model.(Model).Update(space())

	// Apply filter that hides databricks-core.
	model, _ = model.(Model).Update(runes("fast"))
	// databricks-core not visible — no [x] in view right now.

	// Clear filter — should be selected again.
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(model.(Model).View(), "[x]") {
		t.Errorf("selection should survive filter that hides the skill:\n%s", model.(Model).View())
	}
}

func TestStatusLineShowsSelectionCount(t *testing.T) {
	m := loaded(testGroups)
	var model tea.Model = m

	// Select first skill (databricks-core).
	model, _ = model.(Model).Update(space())
	// Move to second skill and select it.
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	model, _ = model.(Model).Update(space())

	if !strings.Contains(model.(Model).View(), "2 selected") {
		t.Errorf("status should show '2 selected':\n%s", model.(Model).View())
	}

	// Deselect one.
	model, _ = model.(Model).Update(space())
	if !strings.Contains(model.(Model).View(), "1 selected") {
		t.Errorf("status should show '1 selected' after deselecting one:\n%s", model.(Model).View())
	}

	// Deselect last (move back to first and deselect).
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyUp})
	model, _ = model.(Model).Update(space())
	out := model.(Model).View()
	if strings.Contains(out, "selected") {
		t.Errorf("status should not mention 'selected' when nothing is selected:\n%s", out)
	}
}

// --- viewport tests ---

// loadedWithHeight returns a loaded Model with a specific terminal height so
// viewport behaviour can be tested without a real terminal.
func loadedWithHeight(groups func() []finder.SkillGroup, height int) Model {
	m := loaded(groups)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: height})
	return next.(Model)
}

func TestViewportScrollDown(t *testing.T) {
	// 3 entries, height=8 → listHeight=2, so only 2 rows fit at a time.
	m := loadedWithHeight(testGroups, 8)
	if m.listHeight() != 2 {
		t.Fatalf("listHeight = %d, want 2", m.listHeight())
	}

	// Move cursor past the viewport bottom (index 0+1 = 1, then 2).
	var model tea.Model = m
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor=1
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown}) // cursor=2, past lh=2

	got := model.(Model).offset
	if got == 0 {
		t.Errorf("offset = 0, want > 0 after scrolling past viewport bottom")
	}
	// Cursor must always be within [offset, offset+lh).
	lh := model.(Model).listHeight()
	off := model.(Model).cursor
	if off < got || off >= got+lh {
		t.Errorf("cursor %d not within viewport [%d, %d)", off, got, got+lh)
	}
}

func TestViewportScrollUp(t *testing.T) {
	m := loadedWithHeight(testGroups, 8) // listHeight=2

	// Scroll down to last entry first.
	var model tea.Model = m
	for i := 0; i < 3; i++ {
		model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	// Now scroll back up to top.
	for i := 0; i < 3; i++ {
		model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyUp})
	}

	if got := model.(Model).offset; got != 0 {
		t.Errorf("offset = %d, want 0 after scrolling back to top", got)
	}
	if got := model.(Model).cursor; got != 0 {
		t.Errorf("cursor = %d, want 0 after scrolling back to top", got)
	}
}

func TestViewportOffsetClampedOnFilter(t *testing.T) {
	// Start with 3 entries in a small viewport, scroll to bottom, then filter
	// down to 1 entry — offset must clamp so no blank rows appear.
	m := loadedWithHeight(testGroups, 8) // listHeight=2

	var model tea.Model = m
	// Move to last entry (index 2 of 3).
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyDown})

	// Apply filter that leaves only 1 match.
	model, _ = model.(Model).Update(runes("fast"))

	got := model.(Model).offset
	visible := model.(Model).selectableCount()
	lh := model.(Model).listHeight()
	maxOk := visible - lh
	if maxOk < 0 {
		maxOk = 0
	}
	if got > maxOk {
		t.Errorf("offset = %d, want <= %d (no blank rows above list)", got, maxOk)
	}
}

func TestStatusLineShowsPosition(t *testing.T) {
	m := loaded(testGroups)
	out := m.View()

	// Cursor starts at 0 → 1-indexed = 1; 3 total entries.
	if !strings.Contains(out, "1/3") {
		t.Errorf("status should show 1/3 on load:\n%s", out)
	}

	// No-match filter → "0 results".
	next, _ := m.Update(runes("zzz"))
	if !strings.Contains(next.(Model).View(), "0 results") {
		t.Errorf("status should show '0 results' when filter matches nothing:\n%s", next.(Model).View())
	}
}

// --- teatest integration tests ---

func TestQuitOnQ(t *testing.T) {
	tm := teatest.NewTestModel(t, loaded(testGroups))
	tm.Send(runes("q"))
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestQuitOnCtrlC(t *testing.T) {
	tm := teatest.NewTestModel(t, loaded(testGroups))
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestTypeToFilterEndToEnd(t *testing.T) {
	tm := teatest.NewTestModel(t, New(testGroups, fakeHome, fakeCwd),
		teatest.WithInitialTermSize(80, 24))

	tm.Type("fast")

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("fastapi")) &&
			!bytes.Contains(b, []byte("databricks-core"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestLoadingThenResults(t *testing.T) {
	release := make(chan struct{})
	blocking := func() []finder.SkillGroup {
		<-release
		return testGroups()
	}

	tm := teatest.NewTestModel(t, New(blocking, fakeHome, fakeCwd),
		teatest.WithInitialTermSize(80, 24))

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("Searching"))
	}, teatest.WithDuration(2*time.Second))

	close(release)

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return bytes.Contains(b, []byte("fastapi"))
	}, teatest.WithDuration(3*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

// --- delete action tests ---

func TestDeleteNoOpWhenNothingSelected(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(runes("D"))
	out := next.(Model).View()
	// Nothing selected — D should not enter confirm state.
	if strings.Contains(out, "Delete") {
		t.Errorf("pressing D with nothing selected should be a no-op, got:\n%s", out)
	}
	if next.(Model).mode != modeNormal {
		t.Errorf("mode should remain modeNormal after no-op D, got %v", next.(Model).mode)
	}
}

func TestDeleteEntersConfirmState(t *testing.T) {
	m := loaded(testGroups)
	var model tea.Model = m
	model, _ = model.(Model).Update(space()) // select cursor row
	model, _ = model.(Model).Update(runes("D"))
	out := model.(Model).View()
	if !strings.Contains(out, "Delete") {
		t.Errorf("pressing D with selection should show delete confirmation:\n%s", out)
	}
	if model.(Model).mode != modeConfirmDelete {
		t.Errorf("mode should be modeConfirmDelete, got %v", model.(Model).mode)
	}
}

func TestDeleteCancelledByOtherKey(t *testing.T) {
	m := loaded(testGroups)
	var model tea.Model = m
	model, _ = model.(Model).Update(space()) // select cursor row
	model, _ = model.(Model).Update(runes("D"))
	// Cancel with esc.
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.(Model).mode != modeNormal {
		t.Errorf("mode should return to modeNormal after cancel, got %v", model.(Model).mode)
	}
	if len(model.(Model).entries) != 3 {
		t.Errorf("entries should be unchanged after cancel, got %d", len(model.(Model).entries))
	}
	// Selection should still be present.
	if len(model.(Model).selected) == 0 {
		t.Errorf("selection should be unchanged after cancel")
	}
}

func TestDeleteConfirmedRemovesEntries(t *testing.T) {
	// Create a real temp dir with a skill subdirectory.
	tmp := t.TempDir()
	skillName := "test-skill"
	skillDir := filepath.Join(tmp, skillName)
	if err := os.Mkdir(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# test"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	groups := func() []finder.SkillGroup {
		return []finder.SkillGroup{
			{Source: tmp, Skills: []string{skillName}},
		}
	}
	m := loaded(groups)
	if len(m.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(m.entries))
	}

	var model tea.Model = m
	model, _ = model.(Model).Update(space())     // select the skill
	model, _ = model.(Model).Update(runes("D"))  // enter confirm
	model, _ = model.(Model).Update(runes("D"))  // confirm delete

	if len(model.(Model).entries) != 0 {
		t.Errorf("entry should be removed from model after delete, got %d entries", len(model.(Model).entries))
	}
	if len(model.(Model).selected) != 0 {
		t.Errorf("selection should be cleared after delete, got %d selected", len(model.(Model).selected))
	}
	if model.(Model).mode != modeNormal {
		t.Errorf("mode should return to modeNormal after delete, got %v", model.(Model).mode)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Errorf("skill directory should have been deleted from filesystem")
	}
}

// --- copy action tests ---

func TestCopyEntersDestPrompt(t *testing.T) {
	m := loaded(testGroups)
	next, _ := m.Update(runes("C"))
	out := next.(Model).View()
	if !strings.Contains(out, "Copy to") {
		t.Errorf("pressing C should show copy destination prompt:\n%s", out)
	}
	if next.(Model).mode != modeChooseCopyDest {
		t.Errorf("mode should be modeChooseCopyDest, got %v", next.(Model).mode)
	}
}

func TestCopyCancelledByOtherKey(t *testing.T) {
	m := loaded(testGroups)
	var model tea.Model = m
	model, _ = model.(Model).Update(runes("C"))
	model, _ = model.(Model).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if model.(Model).mode != modeNormal {
		t.Errorf("mode should return to modeNormal after cancel, got %v", model.(Model).mode)
	}
}

func TestCopyUsesSelectionWhenPresent(t *testing.T) {
	// Set up a real source skill dir.
	srcRoot := t.TempDir()
	dstRoot := t.TempDir()
	skillName := "my-skill"
	if err := os.Mkdir(filepath.Join(srcRoot, skillName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, skillName, "skill.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	groups := func() []finder.SkillGroup {
		return []finder.SkillGroup{{Source: srcRoot, Skills: []string{skillName}}}
	}
	m := New(groups, fakeHome, dstRoot)
	next, _ := m.Update(foundMsg{entries: flattenGroups(groups(), fakeHome)})
	m = next.(Model)

	var model tea.Model = m
	model, _ = model.(Model).Update(space())    // select the skill
	model, _ = model.(Model).Update(runes("C")) // enter copy prompt
	model, _ = model.(Model).Update(runes("1")) // copy to .agents/skills

	dst := filepath.Join(dstRoot, ".agents", "skills", skillName)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		t.Errorf("expected skill to be copied to %s", dst)
	}
	if model.(Model).mode != modeNormal {
		t.Errorf("mode should return to modeNormal after copy, got %v", model.(Model).mode)
	}
}

func TestCopyUsesCursorRowWhenNothingSelected(t *testing.T) {
	srcRoot := t.TempDir()
	dstRoot := t.TempDir()
	skillName := "cursor-skill"
	if err := os.Mkdir(filepath.Join(srcRoot, skillName), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcRoot, skillName, "skill.md"), []byte("# hi"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	groups := func() []finder.SkillGroup {
		return []finder.SkillGroup{{Source: srcRoot, Skills: []string{skillName}}}
	}
	m := New(groups, fakeHome, dstRoot)
	next, _ := m.Update(foundMsg{entries: flattenGroups(groups(), fakeHome)})
	m = next.(Model)

	// No selection — cursor row should be used.
	var model tea.Model = m
	model, _ = model.(Model).Update(runes("C")) // enter copy prompt
	model, _ = model.(Model).Update(runes("2")) // copy to .claude/skills

	dst := filepath.Join(dstRoot, ".claude", "skills", skillName)
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		t.Errorf("expected cursor-row skill to be copied to %s", dst)
	}
}

func TestCopySkipsExistingDestination(t *testing.T) {
	srcRoot := t.TempDir()
	dstRoot := t.TempDir()
	skillName := "existing-skill"
	if err := os.Mkdir(filepath.Join(srcRoot, skillName), 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}

	// Pre-create the destination directory.
	dstSkillDir := filepath.Join(dstRoot, ".agents", "skills", skillName)
	if err := os.MkdirAll(dstSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}

	groups := func() []finder.SkillGroup {
		return []finder.SkillGroup{{Source: srcRoot, Skills: []string{skillName}}}
	}
	m := New(groups, fakeHome, dstRoot)
	next, _ := m.Update(foundMsg{entries: flattenGroups(groups(), fakeHome)})
	m = next.(Model)

	// Copy should skip without error.
	var model tea.Model = m
	model, _ = model.(Model).Update(runes("C"))
	model, _ = model.(Model).Update(runes("1"))

	if model.(Model).mode != modeNormal {
		t.Errorf("mode should return to modeNormal even when dst exists, got %v", model.(Model).mode)
	}
}
