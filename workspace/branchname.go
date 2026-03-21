package workspace

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// ResolveBranchName determines the branch name for a worktree.
// If scriptPath is non-empty, it runs the script with taskJSON on stdin
// and uses the stdout as the branch name. Otherwise returns defaultBranch.
func ResolveBranchName(scriptPath string, taskJSON []byte, defaultBranch string) (string, error) {
	if scriptPath == "" {
		return defaultBranch, nil
	}

	cmd := exec.Command(scriptPath)
	cmd.Stdin = bytes.NewReader(taskJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("branch name script %s: %w\n%s", scriptPath, err, stderr.String())
	}

	name := strings.TrimSpace(stdout.String())
	if name == "" {
		return "", fmt.Errorf("branch name script %s returned empty output", scriptPath)
	}
	return name, nil
}
