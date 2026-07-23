package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/NeerajG03/JEFF/stats"
	"github.com/spf13/cobra"
)

func statsCmd() *cobra.Command {
	var (
		sinceArg   string
		personaArg string
		repoArg    string
		outcomeArg string
		jsonOut    bool
	)

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Aggregate stats over closed/cancelled gig tasks",
		Long:  "Report task throughput, cycle times, and memory/skills usage by persona, repo, and outcome.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var since time.Time
			if sinceArg != "" {
				var err error
				since, err = stats.ParseSince(sinceArg)
				if err != nil {
					return err
				}
			}

			store, err := openGigStore()
			if err != nil {
				return err
			}
			defer store.Close()

			report, err := stats.Collect(store, stats.Options{
				Since:   since,
				Persona: personaArg,
				Repo:    repoArg,
				Outcome: outcomeArg,
			})
			if err != nil {
				return fmt.Errorf("collect stats: %w", err)
			}

			if jsonOut {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}

			fmt.Fprintf(os.Stdout, "JEFF stats since %s\n\n", report.Since.Format(time.RFC3339))
			fmt.Fprintf(os.Stdout, "Tasks: %d\n", len(report.Tasks))
			fmt.Fprintf(os.Stdout, "Memory use: %d/%d\n", report.MemoryUse.WithMemory, report.MemoryUse.Total)
			fmt.Fprintf(os.Stdout, "Skills use: %d/%d\n\n", report.MemoryUse.WithSkills, report.MemoryUse.Total)

			fmt.Fprintln(os.Stdout, "By persona:")
			for _, g := range stats.SortGroups(report.ByPersona) {
				fmt.Fprintf(os.Stdout, "  %s: %d tasks, avg cycle %s\n", g.Name, g.Value.Tasks, fmtDuration(g.Value.AvgCycleTime))
			}

			fmt.Fprintln(os.Stdout, "By repo:")
			for _, g := range stats.SortGroups(report.ByRepo) {
				fmt.Fprintf(os.Stdout, "  %s: %d tasks, avg cycle %s\n", g.Name, g.Value.Tasks, fmtDuration(g.Value.AvgCycleTime))
			}

			fmt.Fprintln(os.Stdout, "By outcome:")
			var outcomeNames []string
			for name := range report.ByOutcome {
				outcomeNames = append(outcomeNames, name)
			}
			sort.Strings(outcomeNames)
			for _, name := range outcomeNames {
				fmt.Fprintf(os.Stdout, "  %s: %d\n", name, report.ByOutcome[name])
			}
			if len(report.ByOutcome) == 0 {
				fmt.Fprintln(os.Stdout, "  (none)")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&sinceArg, "since", "30d", "Window as Nd (e.g. 7d, 30d)")
	cmd.Flags().StringVar(&personaArg, "persona", "", "Filter by persona")
	cmd.Flags().StringVar(&repoArg, "repo", "", "Filter by repo")
	cmd.Flags().StringVar(&outcomeArg, "outcome", "", "Filter by outcome")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output JSON")

	return cmd
}

func fmtDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		return d.Round(time.Minute).String()
	}
	return d.Round(time.Hour).String()
}
