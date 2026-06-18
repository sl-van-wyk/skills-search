// Command skills-search is a terminal UI for browsing agent skills discovered
// under the user's home directory.
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sl-van-wyk/skills-search/finder"
	"github.com/sl-van-wyk/skills-search/ui"
)

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "skills-search: cannot determine home directory:", err)
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "skills-search: cannot determine working directory:", err)
		os.Exit(1)
	}

	model := ui.New(func() []finder.SkillGroup {
		return finder.Walk(home)
	}, home, cwd)

	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "skills-search:", err)
		os.Exit(1)
	}
}
