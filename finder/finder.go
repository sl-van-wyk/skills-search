// Package finder discovers agent skill directories under a root path.
//
// A "skill" is an immediate subdirectory of any directory literally named
// "skills". Skills are grouped by the source "skills" directory that contains
// them. Known high-noise directories (caches, virtualenvs, etc.) are skipped.
package finder

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// SkillGroup is the set of skills found under a single "skills" directory.
type SkillGroup struct {
	// Source is the path to the "skills" directory.
	Source string
	// Skills are the names of the immediate subdirectories, sorted.
	Skills []string
}

// skipDirs are directory base names the walk never descends into, at any depth.
// These hold dependency caches and build artifacts that bury irrelevant
// "skills" directories and slow the walk considerably.
var skipDirs = map[string]struct{}{
	".cache":        {},
	".venv":         {},
	"node_modules":  {},
	"Library":       {},
	"Applications":  {},
	"go":            {},
	".Trash":        {},
	".git":          {},
	".docker":       {},
	".npm":          {},
	".yarn":         {},
	".gem":          {},
	"__pycache__":   {},
	"site-packages": {},
}

// Walk recursively scans root and returns the skill groups it finds. Every
// directory named "skills" becomes one group whose Skills are its immediate
// subdirectories. Directories whose base name is in the skip-list are not
// descended into. Results are sorted by Source, and skills within each group
// are sorted alphabetically, so output is stable across calls.
//
// I/O errors on individual entries are skipped rather than aborting the walk;
// a single unreadable directory should not prevent discovery elsewhere.
func Walk(root string) []SkillGroup {
	var groups []SkillGroup

	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}

		if _, skip := skipDirs[d.Name()]; skip {
			return filepath.SkipDir
		}

		if d.Name() == "skills" {
			if group, ok := readGroup(path); ok {
				groups = append(groups, group)
			}
			// Don't descend further: a "skills" directory's children are skill
			// names, not more "skills" directories to discover.
			return filepath.SkipDir
		}

		return nil
	})

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Source < groups[j].Source
	})

	return groups
}

// readGroup reads the immediate subdirectories of a "skills" directory. It
// returns ok=false when the directory cannot be read or has no subdirectories.
func readGroup(path string) (SkillGroup, bool) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return SkillGroup{}, false
	}

	var skills []string
	for _, e := range entries {
		if e.IsDir() {
			skills = append(skills, e.Name())
		}
	}
	if len(skills) == 0 {
		return SkillGroup{}, false
	}

	sort.Strings(skills)
	return SkillGroup{Source: path, Skills: skills}, true
}
