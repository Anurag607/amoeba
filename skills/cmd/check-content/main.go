// Quick sanity check: load all SKILL.md files under skills/content
// and print id, name, category, priority.
// Run with: go run ./skills/cmd/check-content
package main

import (
	"fmt"
	"os"

	"github.com/anurgosw/agentic-moe/skills"
)

func main() {
	reg := skills.NewRegistry()
	if err := skills.LoadFS(reg, os.DirFS("skills/content"), "."); err != nil {
		fmt.Fprintln(os.Stderr, "load error:", err)
		os.Exit(1)
	}
	for _, s := range reg.All() {
		fmt.Printf("- %-32s [%s] pri=%d  %s\n", s.ID, s.Category, s.Priority, s.Name)
	}
}
