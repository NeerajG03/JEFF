// show_cmd.go — `jeff memory show <id|path>`.
package memory

import (
	"fmt"
	"io"
	"os"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	mem "github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <name|path>",
	Short: "Show a canonical memory entry (frontmatter + body)",
	Long: `Show a memory entry by name or path.

  jeff memory show async-error-handling
  jeff memory show memory/personas/jenko/procedural/async-error-handling.md

If <name> (no slash), searches all scopes for a matching entry.
If <path> (contains slash), reads that file directly.
If multiple entries share the same name, all candidates are listed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := jeff.ResolveHome()
		if err != nil {
			return fmt.Errorf("resolve JEFF_HOME: %w", err)
		}
		return runShow(cmd.OutOrStdout(), home, args[0])
	},
}

func runShow(out io.Writer, home, target string) error {
	if strings.Contains(target, "/") {
		return showByPath(out, target)
	}
	return showByName(out, home, target)
}

func showByPath(out io.Writer, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	_, err = fmt.Fprint(out, string(data))
	return err
}

func showByName(out io.Writer, home, name string) error {
	// Search all statuses (accepted + superseded) so history is visible.
	all, err := mem.ListEntries(home, mem.EntryFilter{})
	if err != nil {
		return err
	}

	var matches []mem.Entry
	for _, e := range all {
		if e.Slug == name || e.FM.Name == name {
			matches = append(matches, e)
		}
	}

	if len(matches) == 0 {
		return fmt.Errorf("no entry found with name %q", name)
	}

	if len(matches) > 1 {
		fmt.Fprintf(out, "Multiple entries found for %q — specify one:\n\n", name)
		for _, m := range matches {
			fmt.Fprintf(out, "  jeff memory show %s\n", m.Path)
		}
		return fmt.Errorf("ambiguous name: %d matches", len(matches))
	}

	return showByPath(out, matches[0].Path)
}
