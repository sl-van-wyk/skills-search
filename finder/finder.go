// Package finder discovers agent skill directories under a root path.
// A skill is an immediate subdirectory of any directory named "skills". Known high-noise directories (caches, virtualenvs, etc.) are skipped.
package finder

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// SkillGroup is the set of skills found under a single "skills" directory.
type SkillGroup struct {
	Source string
	Skills []string
}

// skipDirs are high-noise directory names skipped at any depth (caches, build artifacts).
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

// Walk scans root and returns one SkillGroup per "skills" directory found.
// Directories in skipDirs are not descended. I/O errors on individual entries
// are silently skipped. Output is sorted for stability.
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
			return filepath.SkipDir // children are skill names, not more "skills" dirs
		}

		return nil
	})

	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Source < groups[j].Source
	})

	return groups
}

// readGroup returns the SkillGroup for path; ok=false if unreadable or empty.
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
