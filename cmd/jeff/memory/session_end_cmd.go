// session_end_cmd.go — Worker A: `jeff memory session-end`
// Called by the memory-session-end bash hook when an agent session stops.
package memory

import (
	"fmt"
	"os"
	"strings"

	"github.com/NeerajG03/JEFF/hooks"
	"github.com/spf13/cobra"
)

var sessionEndCmd = &cobra.Command{
	Use:   "session-end",
	Short: "Run memory session-end logic (copy transcript + write queue entry)",
	Long: `Copies the session transcript to JEFF_HOME/transcripts/ and writes a queue
entry to JEFF_HOME/queue/sessions/ for marlowe to process during curate. No LLM
calls are made. Called automatically by the memory-session-end hook on agent Stop.`,
	RunE: runSessionEnd,
}

var (
	seJeffHome    string
	seTask        string
	sePersona     string
	seRepos       string
	seTranscript  string
	seReason      string
	seAgent       string
)

func init() {
	sessionEndCmd.Flags().StringVar(&seJeffHome, "jeff-home", "", "JEFF_HOME path (default: $JEFF_HOME)")
	sessionEndCmd.Flags().StringVar(&seTask, "task", "", "Task ID")
	sessionEndCmd.Flags().StringVar(&sePersona, "persona", "", "Worker persona name")
	sessionEndCmd.Flags().StringVar(&seRepos, "repos", "", "Comma-separated repo names in scope")
	sessionEndCmd.Flags().StringVar(&seTranscript, "transcript", "", "Path to session transcript file")
	sessionEndCmd.Flags().StringVar(&seReason, "reason", "unknown", "Stop reason")
	sessionEndCmd.Flags().StringVar(&seAgent, "agent", "claude", "Agent kind: claude | gemini")
}

func runSessionEnd(cmd *cobra.Command, args []string) error {
	jeffHome := seJeffHome
	if jeffHome == "" {
		jeffHome = os.Getenv("JEFF_HOME")
	}
	if jeffHome == "" {
		return fmt.Errorf("session-end: JEFF_HOME not set (use --jeff-home or $JEFF_HOME)")
	}

	if seTask == "" {
		return fmt.Errorf("session-end: --task is required")
	}

	var repos []string
	if seRepos != "" {
		for _, r := range strings.Split(seRepos, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				repos = append(repos, r)
			}
		}
	}

	return hooks.RunSessionEnd(jeffHome, seTask, sePersona, repos, seAgent, seTranscript, seReason)
}
