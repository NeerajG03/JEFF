// session_start_cmd.go — Worker A: `jeff memory session-start`
// Called by the memory-session-start bash hook at agent session start.
package memory

import (
	"fmt"
	"github.com/NeerajG03/JEFF"
	"os"
	"strings"

	"github.com/NeerajG03/JEFF/hooks"
	"github.com/spf13/cobra"
)

var sessionStartCmd = &cobra.Command{
	Use:   "session-start",
	Short: "Run memory session-start logic (inject addendum + suppress settings)",
	Long: `Ensures the task's CLAUDE.md/GEMINI.md has the memory addendum and the
agent's settings file has native-memory disabled. Idempotent. Called automatically
by the memory-session-start hook at agent session start.`,
	RunE: runSessionStart,
}

var (
	ssJeffHome string
	ssTaskDir  string
	ssPersona  string
	ssTaskID   string
	ssRepos    string
	ssAgent    string
)

func init() {
	sessionStartCmd.Flags().StringVar(&ssJeffHome, "jeff-home", "", "JEFF_HOME path (default: $JEFF_HOME)")
	sessionStartCmd.Flags().StringVar(&ssTaskDir, "task-dir", "", "Task directory (default: current directory)")
	sessionStartCmd.Flags().StringVar(&ssPersona, "persona", "", "Worker persona name")
	sessionStartCmd.Flags().StringVar(&ssTaskID, "task-id", "", "Task ID (e.g. gig-1d33.2)")
	sessionStartCmd.Flags().StringVar(&ssRepos, "repos", "", "Comma-separated repo names in scope")
	sessionStartCmd.Flags().StringVar(&ssAgent, "agent", "claude", fmt.Sprintf("Agent kind: %s", strings.Join(jeff.AgentTool("").ValidNames(), " | ")))
}

func runSessionStart(cmd *cobra.Command, args []string) error {
	jeffHome := ssJeffHome
	if jeffHome == "" {
		jeffHome = os.Getenv("JEFF_HOME")
	}
	if jeffHome == "" {
		return fmt.Errorf("session-start: JEFF_HOME not set (use --jeff-home or $JEFF_HOME)")
	}

	if !jeff.AgentTool(ssAgent).IsValid() {
		return fmt.Errorf("invalid agent %q (must be one of: %s)", ssAgent, strings.Join(jeff.AgentTool("").ValidNames(), ", "))
	}
	taskDir := ssTaskDir
	if taskDir == "" {
		var err error
		taskDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("session-start: getwd: %w", err)
		}
	}

	var repos []string
	if ssRepos != "" {
		for _, r := range strings.Split(ssRepos, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				repos = append(repos, r)
			}
		}
	}

	return hooks.RunSessionStart(jeffHome, taskDir, ssPersona, ssTaskID, repos, ssAgent)
}
