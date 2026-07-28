package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/hooks"
	"github.com/NeerajG03/gig"
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
	RequiredFor string            // empty = all; "crew" for tmux
	Binary      string
	VersionArgs []string
	MinVersion  string
	Install     map[string]string // "darwin" | "linux" | "windows" | "" (fallback)
	OnlyOn      []string          // empty = all platforms
}

type depResult struct {
	dep
	Status  depStatus
	Version string
}

type envStatus string

const (
	envOK   envStatus = "ok"
	envWarn envStatus = "warn"
	envFail envStatus = "fail"
)

type envCheck struct {
	Name     string
	Status   envStatus
	Fix      string
	Required bool
}

type platformInfo struct {
	OS     string `json:"os"`
	Distro string `json:"distro,omitempty"`
}

func detectPlatform() platformInfo {
	p := platformInfo{OS: runtime.GOOS}
	if p.OS == "linux" {
		if d := detectLinuxDistro(); d != "" {
			p.Distro = d
		}
	}
	return p
}

var distroPackageMap = map[string]string{
	"debian":   "apt",
	"ubuntu":   "apt",
	"fedora":   "dnf",
	"rhel":     "dnf",
	"centos":   "dnf",
	"arch":     "pacman",
	"manjaro":  "pacman",
	"alpine":   "apk",
	"opensuse": "zypper",
}

func detectLinuxDistro() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return ""
	}
	defer f.Close()

	var id, idLike string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "ID="):
			id = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		case strings.HasPrefix(line, "ID_LIKE="):
			idLike = strings.Trim(strings.TrimPrefix(line, "ID_LIKE="), `"`)
		}
	}

	if id == "" {
		return ""
	}

	for _, candidate := range []string{id} {
		if _, ok := distroPackageMap[candidate]; ok {
			return candidate
		}
	}
	for _, candidate := range strings.Fields(idLike) {
		if _, ok := distroPackageMap[candidate]; ok {
			return candidate
		}
	}
	return id
}

var linuxPackageMap = map[string]string{
	"git":  "git",
	"jq":   "jq",
	"tmux": "tmux",
	"gh":   "gh",
}

func packageManager(distro string) string {
	if pm, ok := distroPackageMap[distro]; ok {
		return pm
	}
	return ""
}

func formatInstallCommand(pm, pkg string) string {
	switch pm {
	case "pacman":
		return fmt.Sprintf("sudo pacman -S %s", pkg)
	case "apk":
		return fmt.Sprintf("sudo apk add %s", pkg)
	case "zypper":
		return fmt.Sprintf("sudo zypper install %s", pkg)
	default:
		return fmt.Sprintf("sudo %s install %s", pm, pkg)
	}
}

func getDoctorDeps() []dep {
	base := []dep{
		{
			Name:        "gig",
			Required:    true,
			Binary:      "gig",
			VersionArgs: []string{"--version"},
			Install: map[string]string{
				"darwin":  "brew install neerajg03/tap/gig",
				"linux":   "go install github.com/NeerajG03/gig/cmd/gig@latest",
				"windows": "go install github.com/NeerajG03/gig/cmd/gig@latest",
				"":        "go install github.com/NeerajG03/gig/cmd/gig@latest",
			},
		},
		{
			Name:        "tmux",
			Required:    false,
			RequiredFor: "crew",
			Binary:      "tmux",
			VersionArgs: []string{"-V"},
			MinVersion:  "3.0",
			Install: map[string]string{
				"darwin": "brew install tmux",
				"":       "brew install tmux",
			},
		},
		{
			Name:        "git",
			Required:    true,
			Binary:      "git",
			VersionArgs: []string{"--version"},
			Install: map[string]string{
				"darwin": "brew install git",
				"":       "brew install git",
			},
		},
		{
			Name:        "terminal-notifier",
			Required:    false,
			Binary:      "terminal-notifier",
			VersionArgs: []string{"-version"},
			OnlyOn:      []string{"darwin"},
			Install: map[string]string{
				"darwin": "brew install terminal-notifier",
			},
		},
		{
			Name:        "gh",
			Required:    false,
			Binary:      "gh",
			VersionArgs: []string{"--version"},
			Install: map[string]string{
				"darwin": "brew install gh",
				"":       "brew install gh",
			},
		},
		{
			Name:        "jq",
			Required:    true,
			Binary:      "jq",
			VersionArgs: []string{"--version"},
			Install: map[string]string{
				"darwin": "brew install jq",
				"":       "brew install jq",
			},
		},
	}

	seen := make(map[string]bool)
	for _, d := range base {
		seen[d.Name] = true
	}

	for _, agent := range jeff.RegisteredAgents() {
		p := jeff.GetProvider(agent)
		if p == nil {
			continue
		}
		for _, adep := range p.DoctorDeps() {
			if !seen[adep.Name] {
				seen[adep.Name] = true
				installMap := map[string]string{}

				switch adep.Name {
				case "claude":
					installMap[""] = "npm install -g @anthropic-ai/claude-code"
				case "gemini":
					installMap[""] = "npm install -g @google/gemini-cli"
				case "opencode":
					installMap[""] = "npm install -g @opencode/cli"
				}

				base = append(base, dep{
					Name:        adep.Name,
					Required:    adep.Required,
					Binary:      adep.Name,
					VersionArgs: []string{"--version"},
					Install:     installMap,
				})
			}
		}
	}
	return base
}

func doctorCmd() *cobra.Command {
	var jsonOut, fix bool

	cmd := &cobra.Command{
		Use:          "doctor",
		Short:        "Check readiness — dependencies, environment, and what to do next",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			platform := detectPlatform()

			if fix {
				return runDoctorFix(cmd, platform)
			}

			jeffCfg := loadConfigSafe()
			deps := filterDepsForPlatform(getDoctorDeps(), platform)
			depResults := checkAllDeps(deps)
			envResults := runEnvironmentChecks(jeffCfg)
			exitCode := computeExitCode(depResults, envResults)

			if jsonOut {
				printDoctorJSONV1(cmd, depResults, envResults, platform)
				os.Exit(exitCode)
			}

			printDoctorTable(cmd, depResults, platform)
			printEnvironmentTable(cmd, envResults)
			printNextStepsRaw(cmd, depResults, envResults, platform)

			os.Exit(exitCode)
			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON (version 1)")
	cmd.Flags().BoolVar(&fix, "fix", false, "Run safe JEFF-side repairs")
	return cmd
}

func loadConfigSafe() *jeff.Config {
	home, err := jeff.ResolveHome()
	if err != nil {
		return nil
	}
	cfg, err := jeff.LoadConfig(home)
	if err != nil {
		return nil
	}
	return cfg
}

func filterDepsForPlatform(deps []dep, platform platformInfo) []dep {
	var out []dep
	for _, d := range deps {
		if len(d.OnlyOn) > 0 {
			found := false
			for _, osName := range d.OnlyOn {
				if osName == platform.OS {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		out = append(out, d)
	}
	return out
}

func installHint(name string, installMap map[string]string, platform platformInfo) string {
	if len(installMap) == 0 {
		return ""
	}

	if platform.OS == "linux" && platform.Distro != "" {
		pm := packageManager(platform.Distro)
		if pm != "" {
			if pkg, ok := linuxPackageMap[name]; ok {
				return formatInstallCommand(pm, pkg)
			}
			if hint, ok := installMap["linux"]; ok {
				return hint
			}
			return fmt.Sprintf("use %s or see https://docs.jeff.ai/requirements", pm)
		}
	}

	if hint, ok := installMap[platform.OS]; ok {
		return hint
	}
	if hint, ok := installMap[""]; ok {
		return hint
	}
	return ""
}

func computeExitCode(depResults []depResult, envResults []envCheck) int {
	hasRequiredFailed := false
	hasWarning := false

	for _, r := range depResults {
		if r.Status != statusOK {
			if r.Required {
				hasRequiredFailed = true
			} else {
				hasWarning = true
			}
		}
	}

	for _, e := range envResults {
		switch e.Status {
		case envFail:
			if e.Required {
				hasRequiredFailed = true
			} else {
				hasWarning = true
			}
		case envWarn:
			hasWarning = true
		}
	}

	if hasRequiredFailed {
		return 1
	}
	if hasWarning {
		return 2
	}
	return 0
}

func runDoctorFix(cmd *cobra.Command, platform platformInfo) error {
	out := cmd.OutOrStdout()

	if err := runUpdate(); err != nil {
		fmt.Fprintf(out, "jeff init --update: %s\n", colorize(cRed, err.Error()))
	} else {
		fmt.Fprintf(out, "jeff init --update: %s\n", colorize(cGreen, "done"))
	}

	jeffCfg := loadConfigSafe()
	if jeffCfg != nil {
		if err := syncHomeHooks(jeffCfg.Home, jeffCfg); err != nil {
			fmt.Fprintf(out, "hook sync: %s\n", colorize(cRed, err.Error()))
		} else {
			fmt.Fprintf(out, "hook sync: %s\n", colorize(cGreen, "done"))
		}

		store, err := openGigStore(jeffCfg)
		if err == nil {
			if err := jeff.EnsureAttrs(store); err != nil {
				fmt.Fprintf(out, "gig EnsureAttrs: %s\n", colorize(cRed, err.Error()))
			} else {
				fmt.Fprintf(out, "gig EnsureAttrs: %s\n", colorize(cGreen, "done"))
			}
			store.Close()
		} else {
			fmt.Fprintf(out, "gig EnsureAttrs: %s (gig store not reachable)\n", colorize(cYellow, "skipped"))
		}
	}

	fmt.Fprintln(out)

	deps := filterDepsForPlatform(getDoctorDeps(), platform)
	depResults := checkAllDeps(deps)
	envResults := runEnvironmentChecks(jeffCfg)

	printDoctorTable(cmd, depResults, platform)
	printEnvironmentTable(cmd, envResults)
	printNextStepsRaw(cmd, depResults, envResults, platform)

	exitCode := computeExitCode(depResults, envResults)
	os.Exit(exitCode)
	return nil
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

func runEnvironmentChecks(cfg *jeff.Config) []envCheck {
	var checks []envCheck

	home, err := jeff.ResolveHome()
	jeffInitialized := err == nil && home != ""
	if jeffInitialized {
		_, err := os.Stat(jeff.ConfigPath(home))
		jeffInitialized = err == nil
	}

	if !jeffInitialized {
		checks = append(checks, envCheck{Name: "jeff_initialized", Status: envFail, Fix: "jeff init", Required: true})
		checks = append(checks, envCheck{Name: "agent_installed", Status: envFail, Fix: "jeff init", Required: true})
		checks = append(checks, envCheck{Name: "repos_configured", Status: envFail, Fix: "jeff init", Required: true})
		checks = append(checks, envCheck{Name: "gig_initialized", Status: envFail, Fix: "jeff init", Required: true})
		checks = append(checks, envCheck{Name: "gh_authenticated", Status: envWarn, Fix: "gh auth login", Required: false})
		checks = append(checks, envCheck{Name: "hooks_in_sync", Status: envFail, Fix: "jeff init", Required: false})
		return checks
	}

	if cfg == nil {
		checks = append(checks, envCheck{Name: "jeff_initialized", Status: envWarn, Fix: "jeff init --update", Required: true})
		checks = append(checks, envCheck{Name: "gig_initialized", Status: envFail, Fix: "jeff init", Required: true})
		checks = append(checks, envCheck{Name: "agent_installed", Status: envFail, Fix: "jeff init", Required: true})
		checks = append(checks, envCheck{Name: "repos_configured", Status: envFail, Fix: "jeff init", Required: true})
		checks = append(checks, envCheck{Name: "gh_authenticated", Status: envWarn, Fix: "gh auth login", Required: false})
		checks = append(checks, envCheck{Name: "hooks_in_sync", Status: envFail, Fix: "jeff init", Required: false})
		return checks
	}

	checks = append(checks, envCheck{Name: "jeff_initialized", Status: envOK, Required: true})

	checks = append(checks, checkGigInitialized(cfg))

	checks = append(checks, checkAgentInstalled(cfg))

	checks = append(checks, checkReposRegistered(cfg))

	checks = append(checks, checkGHAuth())

	checks = append(checks, checkHooksInSync(cfg))

	return checks
}

func checkGigInitialized(cfg *jeff.Config) envCheck {
	if _, err := os.Stat(gig.DefaultConfigPath()); os.IsNotExist(err) {
		return envCheck{Name: "gig_initialized", Status: envFail, Fix: "gig init --prefix <name>", Required: true}
	}
	store, err := openGigStore(cfg)
	if err != nil {
		return envCheck{Name: "gig_initialized", Status: envFail, Fix: "gig init --prefix <name>", Required: true}
	}
	store.Close()
	return envCheck{Name: "gig_initialized", Status: envOK, Required: true}
}

func checkAgentInstalled(cfg *jeff.Config) envCheck {
	c := envCheck{Name: "agent_installed", Required: true}

	agentCmd := string(cfg.Agent)
	if _, err := exec.LookPath(agentCmd); err != nil {
		var installed []string
		for _, agent := range jeff.RegisteredAgents() {
			if _, err := exec.LookPath(string(agent)); err == nil {
				installed = append(installed, string(agent))
			}
		}
		if len(installed) > 0 {
			c.Fix = fmt.Sprintf("jeff config agent %s", installed[0])
		} else {
			c.Fix = "install an agent CLI (e.g. npm install -g @opencode/cli)"
		}
		c.Status = envFail
	} else {
		c.Status = envOK
	}
	return c
}

func checkReposRegistered(cfg *jeff.Config) envCheck {
	c := envCheck{Name: "repos_configured", Required: true}

	if len(jeff.ListRepos(cfg)) == 0 {
		c.Status = envFail
		c.Fix = "jeff repo add <url>"
	} else {
		c.Status = envOK
	}
	return c
}

func checkGHAuth() envCheck {
	c := envCheck{Name: "gh_authenticated", Required: false}

	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	_ = out
	if err != nil {
		c.Status = envWarn
		c.Fix = "gh auth login"
	} else {
		c.Status = envOK
	}
	return c
}

func checkHooksInSync(cfg *jeff.Config) envCheck {
	c := envCheck{Name: "hooks_in_sync", Required: false}

	if hooks.TaskHooksStale(cfg.Home) {
		c.Status = envFail
		c.Fix = "jeff config hooks sync"
	} else {
		c.Status = envOK
	}
	return c
}

func buildNextSteps(depResults []depResult, envResults []envCheck, platform platformInfo) []string {
	var next []string
	seen := make(map[string]bool)

	priorityItems := map[string]int{
		"jeff init": 0,
	}
	var entries []struct {
		step     string
		priority int
	}

	for _, r := range depResults {
		if r.Status != statusOK && r.Required {
			hint := installHint(r.Name, r.Install, platform)
			if hint != "" && !seen[hint] {
				seen[hint] = true
				entries = append(entries, struct {
					step     string
					priority int
				}{hint, 10})
			}
		}
	}

	for _, e := range envResults {
		if e.Status == envFail && e.Fix != "" && !seen[e.Fix] {
			seen[e.Fix] = true
			pri, ok := priorityItems[e.Fix]
			if !ok {
				pri = 5
			}
			entries = append(entries, struct {
				step     string
				priority int
			}{e.Fix, pri})
		}
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].priority < entries[j].priority
	})

	for _, e := range entries {
		next = append(next, e.step)
	}

	return next
}

var versionRE = regexp.MustCompile(`\d+\.\d+(?:\.\d+)*`)

func parseVersion(output string) string {
	return versionRE.FindString(output)
}

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
		i := strings.IndexAny(p, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-")
		if i >= 0 {
			p = p[:i]
		}
		n, _ := strconv.Atoi(p)
		nums = append(nums, n)
	}
	return nums
}

func printDoctorTable(cmd *cobra.Command, results []depResult, platform platformInfo) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, colorize(cBold, "DEPENDENCIES"))
	header := fmt.Sprintf("  %s  %s  %s  %s",
		padRight("BINARY", 20),
		padRight("STATUS", 14),
		padRight("VERSION", 12),
		"INSTALL",
	)
	fmt.Fprintln(out, header)
	fmt.Fprintln(out, "  "+strings.Repeat("-", 63))

	for _, r := range results {
		statusStr := formatDepStatus(r.Status, r.Required, r.RequiredFor)
		version := r.Version
		if version == "" {
			version = "—"
		}
		install := "—"
		if r.Status != statusOK {
			hint := installHint(r.Name, r.Install, platform)
			if hint != "" {
				install = hint
			}
		}
		fmt.Fprintf(out, "  %s  %s  %s  %s\n",
			padRight(r.Name, 20),
			padRight(statusStr, 14),
			padRight(version, 12),
			install,
		)
	}
}

func formatDepStatus(s depStatus, required bool, requiredFor string) string {
	switch s {
	case statusOK:
		return colorize(cGreen, "✓")
	case statusMissing:
		if !required && requiredFor != "" {
			return colorize(cYellow, "⚠ needed for crew")
		}
		return colorize(cRed, "✗ missing")
	case statusOutdated:
		return colorize(cYellow, "⚠ outdated")
	default:
		return string(s)
	}
}

func printEnvironmentTable(cmd *cobra.Command, results []envCheck) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, colorize(cBold, "ENVIRONMENT"))

	for _, e := range results {
		statusStr := formatEnvStatus(e.Status)
		fmt.Fprintf(out, "  %s  %s  %s\n",
			padRight(statusStr, 16),
			padRight(e.Name, 24),
			e.Fix,
		)
	}
}

func formatEnvStatus(s envStatus) string {
	switch s {
	case envOK:
		return colorize(cGreen, "✓ ok")
	case envWarn:
		return colorize(cYellow, "⚠ warn")
	case envFail:
		return colorize(cRed, "✗ fail")
	default:
		return string(s)
	}
}

func printNextStepsRaw(cmd *cobra.Command, depResults []depResult, envResults []envCheck, platform platformInfo) {
	next := buildNextSteps(depResults, envResults, platform)
	if len(next) == 0 {
		return
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out)
	fmt.Fprintln(out, colorize(cBold, "NEXT"))
	for _, step := range next {
		fmt.Fprintf(out, "  $ %s\n", step)
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

type jsonEnvEntry struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Required bool   `json:"required"`
	Fix      string `json:"fix,omitempty"`
}

type jsonDoctorOutputV1 struct {
	Version     int             `json:"version"`
	OK          bool            `json:"ok"`
	Platform    platformInfo    `json:"platform"`
	Deps        []jsonDepEntry  `json:"deps"`
	Environment []jsonEnvEntry  `json:"environment"`
	Next        []string        `json:"next"`
}

func printDoctorJSONV1(cmd *cobra.Command, depResults []depResult, envResults []envCheck, platform platformInfo) {
	hasRequiredFailed := false
	for _, r := range depResults {
		if r.Required && r.Status != statusOK {
			hasRequiredFailed = true
			break
		}
	}
	if !hasRequiredFailed {
		for _, e := range envResults {
			if e.Required && e.Status != envOK {
				hasRequiredFailed = true
				break
			}
		}
	}

	next := buildNextSteps(depResults, envResults, platform)

	if next == nil {
		next = []string{}
	}

	deps := make([]jsonDepEntry, 0, len(depResults))
	env := make([]jsonEnvEntry, 0, len(envResults))

	out := jsonDoctorOutputV1{
		Version:     1,
		OK:          !hasRequiredFailed,
		Platform:    platform,
		Deps:        deps,
		Environment: env,
		Next:        next,
	}

	for _, r := range depResults {
		entry := jsonDepEntry{
			Name:     r.Name,
			Status:   string(r.Status),
			Version:  r.Version,
			Required: r.Required,
		}
		if r.Status != statusOK {
			entry.Install = installHint(r.Name, r.Install, platform)
		}
		out.Deps = append(out.Deps, entry)
	}

	for _, e := range envResults {
		out.Environment = append(out.Environment, jsonEnvEntry{
			Name:     e.Name,
			Status:   string(e.Status),
			Required: e.Required,
			Fix:      e.Fix,
		})
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}
