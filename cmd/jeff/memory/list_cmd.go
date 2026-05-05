// list_cmd.go — `jeff memory list` with scope/bucket/status/limit/json filters.
package memory

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	jeff "github.com/NeerajG03/JEFF"
	mem "github.com/NeerajG03/JEFF/memory"
	"github.com/spf13/cobra"
)

type listOpts struct {
	persona string
	repo    string
	project string
	scope   string
	bucket  string
	status  string
	limit   int
	asJSON  bool
}

var (
	listPersona string
	listRepo    string
	listProject string
	listScope   string
	listBucket  string
	listStatus  string
	listLimit   int
	listJSON    bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List canonical memory entries",
	Long: `List canonical memory entries from JEFF_HOME/memory/.

Defaults to accepted entries only. Use --status superseded to see history.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := jeff.ResolveHome()
		if err != nil {
			return fmt.Errorf("resolve JEFF_HOME: %w", err)
		}
		opts := listOpts{
			persona: listPersona,
			repo:    listRepo,
			project: listProject,
			scope:   listScope,
			bucket:  listBucket,
			status:  listStatus,
			limit:   listLimit,
			asJSON:  listJSON,
		}
		return runList(cmd.OutOrStdout(), home, opts)
	},
}

func init() {
	listCmd.Flags().StringVar(&listPersona, "persona", "", "Filter by persona name (e.g. jenko)")
	listCmd.Flags().StringVar(&listRepo, "repo", "", "Filter by repo name")
	listCmd.Flags().StringVar(&listProject, "project", "", "Filter by project key")
	listCmd.Flags().StringVar(&listScope, "scope", "", "Exact scope (persona:x | repo:y | project:z)")
	listCmd.Flags().StringVar(&listBucket, "bucket", "", "Filter by bucket (core|procedural|semantic|episodic)")
	listCmd.Flags().StringVar(&listStatus, "status", "accepted", "Filter by status (accepted|superseded)")
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "Maximum entries to show (0 = unlimited)")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON array")
}

func runList(out io.Writer, home string, opts listOpts) error {
	filter := mem.EntryFilter{
		Persona: opts.persona,
		Repo:    opts.repo,
		Project: opts.project,
		Status:  opts.status,
	}

	if opts.scope != "" {
		parts := strings.SplitN(opts.scope, ":", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid scope %q: expected kind:name (e.g. persona:jenko)", opts.scope)
		}
		switch parts[0] {
		case "persona":
			filter.Persona = parts[1]
		case "repo":
			filter.Repo = parts[1]
		case "project":
			filter.Project = parts[1]
		default:
			return fmt.Errorf("unknown scope kind %q (want persona|repo|project)", parts[0])
		}
	}

	if opts.bucket != "" {
		b := mem.Bucket(opts.bucket)
		if !b.Valid() {
			return fmt.Errorf("invalid bucket %q (want core|procedural|semantic|episodic)", opts.bucket)
		}
		filter.Bucket = b
	}

	entries, err := mem.ListEntries(home, filter)
	if err != nil {
		return err
	}

	if opts.limit > 0 && len(entries) > opts.limit {
		entries = entries[:opts.limit]
	}

	if len(entries) == 0 {
		fmt.Fprintln(out, "No entries found.")
		return nil
	}

	if opts.asJSON {
		return printListJSON(out, entries)
	}
	return printListTable(out, entries)
}

type listJSONEntry struct {
	Scope       string `json:"scope"`
	Bucket      string `json:"bucket"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Path        string `json:"path"`
}

func printListJSON(out io.Writer, entries []mem.Entry) error {
	rows := make([]listJSONEntry, len(entries))
	for i, e := range entries {
		rows[i] = listJSONEntry{
			Scope:       e.Scope,
			Bucket:      string(e.Bucket),
			Name:        e.FM.Name,
			Description: e.FM.Description,
			Status:      e.FM.Status,
			Path:        e.Path,
		}
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(rows)
}

func printListTable(out io.Writer, entries []mem.Entry) error {
	// Compute column widths.
	scopeW, bucketW, nameW := len("SCOPE"), len("BUCKET"), len("NAME")
	for _, e := range entries {
		if l := len(e.Scope); l > scopeW {
			scopeW = l
		}
		if l := len(string(e.Bucket)); l > bucketW {
			bucketW = l
		}
		if l := len(e.FM.Name); l > nameW {
			nameW = l
		}
	}

	fmt.Fprintf(out, "  %-*s  %-*s  %-*s  %s\n",
		scopeW, "SCOPE",
		bucketW, "BUCKET",
		nameW, "NAME",
		"DESCRIPTION",
	)
	fmt.Fprintln(out, "  "+strings.Repeat("─", scopeW+bucketW+nameW+50))

	for _, e := range entries {
		desc := e.FM.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Fprintf(out, "  %-*s  %-*s  %-*s  %s\n",
			scopeW, e.Scope,
			bucketW, string(e.Bucket),
			nameW, e.FM.Name,
			desc,
		)
	}

	fmt.Fprintf(out, "\n%d entries\n", len(entries))
	return nil
}
