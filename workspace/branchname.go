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

// IsHotfixBase reports whether a base branch's path segments imply a
// hotfix/ branch prefix (gig-0459). Matches whole path segments only,
// case-insensitively, never substrings, so branches that merely CONTAIN
// "hotfix" or "prod" (e.g. "feature/CB-8888_workflow_status_hotfix",
// "cb-14147-prod-ga-id") are not misidentified:
//
//   - any segment equals "hotfix" (catches "hotfix/...", "hotFix/...")
//   - any segment equals "release" (catches "release/1.2.3")
//   - the FINAL segment equals "production" exactly (never bare "prod")
func IsHotfixBase(base string) bool {
	segs := strings.Split(base, "/")
	for i, seg := range segs {
		s := strings.ToLower(seg)
		if s == "hotfix" || s == "release" {
			return true
		}
		if i == len(segs)-1 && s == "production" {
			return true
		}
	}
	return false
}

// InferHotfixBranch returns branch prefixed with "hotfix/" when base
// implies a hotfix branch (IsHotfixBase) and branch doesn't already carry
// the prefix. The second return value reports whether it applied a new
// prefix, so callers can print what they did — branch names end up in
// other people's PRs, so silent renaming is worse than no inference.
func InferHotfixBranch(base, branch string) (string, bool) {
	if !IsHotfixBase(base) || strings.HasPrefix(branch, "hotfix/") {
		return branch, false
	}
	return "hotfix/" + branch, true
}
