package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type depStatus string

const (
	statusOK       depStatus = "ok"
	statusMissing  depStatus = "missing"
	statusOutdated depStatus = "outdated"
)

type dep struct {
	Name        string
	Required    bool
	Binary      string
	VersionArgs []string
	MinVersion  string
	InstallCmd  string
}

type depResult struct {
	dep
	Status  depStatus
	Version string
}

var doctorDeps = []dep{
	{
		Name:        "tmux",
		Required:    true,
		Binary:      "tmux",
		VersionArgs: []string{"-V"},
		MinVersion:  "3.0",
		InstallCmd:  "brew install tmux",
	},
	{
		Name:        "git",
		Required:    true,
		Binary:      "git",
		VersionArgs: []string{"--version"},
		InstallCmd:  "brew install git",
	},
	{
		Name:        "terminal-notifier",
		Required:    false,
		Binary:      "terminal-notifier",
		VersionArgs: []string{"-version"},
		InstallCmd:  "brew install terminal-notifier",
	},
	{
		Name:        "gh",
		Required:    false,
		Binary:      "gh",
		VersionArgs: []string{"--version"},
		InstallCmd:  "brew install gh",
	},
	{
		Name:        "claude",
		Required:    false,
		Binary:      "claude",
		VersionArgs: []string{"--version"},
		InstallCmd:  "npm install -g @anthropic-ai/claude-code",
	},
	{
		Name:        "gemini",
		Required:    false,
		Binary:      "gemini",
		VersionArgs: []string{"--version"},
		InstallCmd:  "npm install -g @google/gemini-cli",
	},
}

func doctorCmd() *cobra.Command {
	var jsonOut bool

	cmd := &cobra.Command{
		Use:          "doctor",
		Short:        "Check required and optional dependencies",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			results := checkAllDeps(doctorDeps)

			anyRequiredFailed := false
			for _, r := range results {
				if r.Required && r.Status != statusOK {
					anyRequiredFailed = true
					break
				}
			}

			if jsonOut {
				if err := printDoctorJSON(cmd, results, anyRequiredFailed); err != nil {
					return err
				}
				if anyRequiredFailed {
					return fmt.Errorf("required dependencies are missing or outdated")
				}
				return nil
			}

			printDoctorTable(cmd, results)
			if anyRequiredFailed {
				return fmt.Errorf("required dependencies are missing or outdated")
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func checkAllDeps(deps []dep) []depResult {
	results := make([]depResult, len(deps))
	for i, d := range deps {
		results[i] = checkDep(d)
	}
	return results
}

func checkDep(d dep) depResult {
	r := depResult{dep: d}

	if _, err := exec.LookPath(d.Binary); err != nil {
		r.Status = statusMissing
		return r
	}

	if len(d.VersionArgs) > 0 {
		out, err := exec.Command(d.Binary, d.VersionArgs...).CombinedOutput()
		if err == nil {
			r.Version = parseVersion(string(out))
		}
	}

	if d.MinVersion != "" && r.Version != "" && versionLess(r.Version, d.MinVersion) {
		r.Status = statusOutdated
		return r
	}

	r.Status = statusOK
	return r
}

var versionRE = regexp.MustCompile(`\d+\.\d+(?:\.\d+)*`)

func parseVersion(output string) string {
	return versionRE.FindString(output)
}

// versionLess returns true if semver string a is less than b.
func versionLess(a, b string) bool {
	ap := splitVersion(a)
	bp := splitVersion(b)

	maxLen := len(ap)
	if len(bp) > maxLen {
		maxLen = len(bp)
	}

	for i := 0; i < maxLen; i++ {
		av, bv := 0, 0
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av < bv {
			return true
		}
		if av > bv {
			return false
		}
	}
	return false
}

func splitVersion(v string) []int {
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		// Strip non-numeric suffix (e.g. "3.4a" → 4)
		i := strings.IndexAny(p, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-")
		if i >= 0 {
			p = p[:i]
		}
		n, _ := strconv.Atoi(p)
		nums = append(nums, n)
	}
	return nums
}

func printDoctorTable(cmd *cobra.Command, results []depResult) {
	out := cmd.OutOrStdout()
	header := fmt.Sprintf("%s  %s  %s  %s",
		padRight("DEP", 20),
		padRight("STATUS", 14),
		padRight("VERSION", 12),
		"INSTALL",
	)
	fmt.Fprintln(out, header)
	fmt.Fprintln(out, strings.Repeat("-", 65))

	for _, r := range results {
		statusStr := formatDepStatus(r.Status)
		version := r.Version
		if version == "" {
			version = "—"
		}
		install := "—"
		if r.Status != statusOK && r.InstallCmd != "" {
			install = r.InstallCmd
		}
		fmt.Fprintf(out, "%s  %s  %s  %s\n",
			padRight(r.Name, 20),
			padRight(statusStr, 14),
			padRight(version, 12),
			install,
		)
	}
}

func formatDepStatus(s depStatus) string {
	switch s {
	case statusOK:
		return colorize(cGreen, "✓")
	case statusMissing:
		return colorize(cRed, "✗ missing")
	case statusOutdated:
		return colorize(cYellow, "⚠ outdated")
	default:
		return string(s)
	}
}

func padRight(s string, width int) string {
	vl := visibleLen(s)
	if vl >= width {
		return s
	}
	return s + strings.Repeat(" ", width-vl)
}

type jsonDepEntry struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Version  string `json:"version,omitempty"`
	Required bool   `json:"required"`
	Install  string `json:"install,omitempty"`
}

type jsonDoctorOutput struct {
	OK   bool           `json:"ok"`
	Deps []jsonDepEntry `json:"deps"`
}

func printDoctorJSON(cmd *cobra.Command, results []depResult, anyRequiredFailed bool) error {
	out := jsonDoctorOutput{
		OK:   !anyRequiredFailed,
		Deps: make([]jsonDepEntry, 0, len(results)),
	}
	for _, r := range results {
		entry := jsonDepEntry{
			Name:     r.Name,
			Status:   string(r.Status),
			Version:  r.Version,
			Required: r.Required,
		}
		if r.Status != statusOK && r.InstallCmd != "" {
			entry.Install = r.InstallCmd
		}
		out.Deps = append(out.Deps, entry)
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
