// diff_cmd.go — `jeff memory diff <name>` — walks the supersedes/superseded_by chain.
package memory

import (
	"fmt"
	"io"
	"sort"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	mem "github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

var diffCmd = &cobra.Command{
	Use:   "diff <name>",
	Short: "Show bi-temporal history for a memory entry (supersedes chain)",
	Long: `Walk the supersedes/superseded_by chain for a named memory entry and display
the full version history with valid windows.

Example:
  jeff memory diff async-error-handling`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := jeff.ResolveHome()
		if err != nil {
			return fmt.Errorf("resolve JEFF_HOME: %w", err)
		}
		return runDiff(cmd.OutOrStdout(), home, args[0])
	},
}

func runDiff(out io.Writer, home, name string) error {
	// Load all entries across all statuses to capture history.
	all, err := mem.ListEntries(home, mem.EntryFilter{})
	if err != nil {
		return err
	}

	chain := buildDiffChain(all, name)
	if len(chain) == 0 {
		return fmt.Errorf("no entry found with name %q", name)
	}

	fmt.Fprintln(out, name)
	fmt.Fprintln(out, strings.Repeat("─", len(name)+2))

	for i, e := range chain {
		version := fmt.Sprintf("v%d", i+1)
		validTo := "present"
		if e.FM.ValidTo != nil {
			validTo = e.FM.ValidTo.Format("2006-01-02")
		}
		validFrom := "?"
		if !e.FM.ValidFrom.IsZero() {
			validFrom = e.FM.ValidFrom.Format("2006-01-02")
		}

		current := ""
		if e.FM.Status == "accepted" {
			current = "   ← current"
		}

		fmt.Fprintf(out, "\n%s (valid %s → %s)%s\n", version, validFrom, validTo, current)
		fmt.Fprintf(out, "   scope:  %s / %s\n", e.Scope, e.Bucket)

		if e.FM.Description != "" {
			fmt.Fprintf(out, "   %s\n", e.FM.Description)
		}

		if e.FM.SupersededBy != "" {
			fmt.Fprintf(out, "   [superseded by: %s]\n", e.FM.SupersededBy)
		}
		if len(e.FM.Supersedes) > 0 {
			fmt.Fprintf(out, "   [supersedes: %s]\n", strings.Join(e.FM.Supersedes, ", "))
		}
	}

	fmt.Fprintln(out)
	return nil
}

// buildDiffChain collects all entries in the supersedes/superseded_by chain
// rooted at startName, sorted by valid_from ascending.
func buildDiffChain(entries []mem.Entry, startName string) []mem.Entry {
	// Build slug→entry index. For duplicate slugs (different scopes), prefer accepted.
	bySlug := make(map[string]mem.Entry)
	for _, e := range entries {
		existing, ok := bySlug[e.Slug]
		if !ok {
			bySlug[e.Slug] = e
			continue
		}
		// Prefer accepted over superseded when slugs collide across scopes.
		if e.FM.Status == "accepted" && existing.FM.Status != "accepted" {
			bySlug[e.Slug] = e
		}
	}

	visited := map[string]bool{}
	var chain []mem.Entry

	var collect func(slug string)
	collect = func(slug string) {
		if visited[slug] {
			return
		}
		visited[slug] = true
		e, ok := bySlug[slug]
		if !ok {
			return
		}
		chain = append(chain, e)
		for _, s := range e.FM.Supersedes {
			collect(s)
		}
		if e.FM.SupersededBy != "" {
			collect(e.FM.SupersededBy)
		}
	}

	collect(startName)

	sort.Slice(chain, func(i, j int) bool {
		ti, tj := chain[i].FM.ValidFrom, chain[j].FM.ValidFrom
		if ti.IsZero() && tj.IsZero() {
			return chain[i].Slug < chain[j].Slug
		}
		if ti.IsZero() {
			return true
		}
		if tj.IsZero() {
			return false
		}
		return ti.Before(tj)
	})

	return chain
}

