package finder

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// mkdirs creates each given directory (relative to root) including parents.
func mkdirs(t *testing.T, root string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("MkdirAll %q: %v", d, err)
		}
	}
}

func TestWalk(t *testing.T) {
	tests := []struct {
		name string
		// dirs to create relative to the temp root.
		dirs []string
		// want maps a source path (relative to root) to its expected skills.
		want map[string][]string
	}{
		{
			name: "single source with skills",
			dirs: []string{".claude/skills/fastapi", ".claude/skills/databricks-core"},
			want: map[string][]string{
				".claude/skills": {"databricks-core", "fastapi"},
			},
		},
		{
			name: "multiple sources",
			dirs: []string{".claude/skills/fastapi", ".agents/skills/openspec-explore"},
			want: map[string][]string{
				".claude/skills": {"fastapi"},
				".agents/skills": {"openspec-explore"},
			},
		},
		{
			name: "no skills directories",
			dirs: []string{".claude/config", "projects/foo"},
			want: map[string][]string{},
		},
		{
			name: "skip-listed directory is not walked",
			dirs: []string{".cache/uv/skills/some-skill", ".claude/skills/bar"},
			want: map[string][]string{
				".claude/skills": {"bar"},
			},
		},
		{
			name: "empty skills directory yields no group",
			dirs: []string{".claude/skills"},
			want: map[string][]string{},
		},
		{
			name: "skills entries that are files are ignored",
			dirs: []string{".claude/skills/real-skill"},
			want: map[string][]string{
				".claude/skills": {"real-skill"},
			},
		},
		{
			name: "nested project-local skills are found",
			dirs: []string{"projects/tutor/.claude/skills/openspec-propose"},
			want: map[string][]string{
				"projects/tutor/.claude/skills": {"openspec-propose"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			mkdirs(t, root, tt.dirs...)

			// For the "files are ignored" case, drop a plain file alongside.
			if tt.name == "skills entries that are files are ignored" {
				f := filepath.Join(root, ".claude/skills/README.md")
				if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			got := Walk(root)

			// Build a comparable map from results, rebasing Source onto root.
			gotMap := make(map[string][]string, len(got))
			for _, g := range got {
				rel, err := filepath.Rel(root, g.Source)
				if err != nil {
					t.Fatalf("Rel: %v", err)
				}
				gotMap[rel] = g.Skills
			}

			if !reflect.DeepEqual(gotMap, tt.want) {
				t.Errorf("Walk() = %v, want %v", gotMap, tt.want)
			}
		})
	}
}

func TestWalkSortsGroupsBySource(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root,
		"zeta/skills/a",
		"alpha/skills/b",
		"mid/skills/c",
	)

	got := Walk(root)
	if len(got) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(got))
	}

	for i := 1; i < len(got); i++ {
		if got[i-1].Source >= got[i].Source {
			t.Errorf("groups not sorted by Source: %q before %q", got[i-1].Source, got[i].Source)
		}
	}
}

func TestWalkStableAcrossCalls(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root,
		".claude/skills/fastapi",
		".claude/skills/shadcn",
		".agents/skills/openspec-explore",
	)

	first := Walk(root)
	second := Walk(root)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("Walk not stable across calls:\nfirst  = %v\nsecond = %v", first, second)
	}
}

func TestWalkSkillsSortedWithinGroup(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root,
		".claude/skills/zebra",
		".claude/skills/apple",
		".claude/skills/mango",
	)

	got := Walk(root)
	if len(got) != 1 {
		t.Fatalf("expected 1 group, got %d", len(got))
	}

	want := []string{"apple", "mango", "zebra"}
	if !reflect.DeepEqual(got[0].Skills, want) {
		t.Errorf("skills = %v, want %v", got[0].Skills, want)
	}
}
