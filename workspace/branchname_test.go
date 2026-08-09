package workspace

import "testing"

// TestIsHotfixBase pins the segment-matching rule for gig-0459 against real
// branch names pulled from the registered repos (backend/frontend/python-
// backend) — chosen because a naive substring match ("contains hotfix" or
// "contains prod") misfires on several of them.
func TestIsHotfixBase(t *testing.T) {
	tests := []struct {
		base string
		want bool
	}{
		// Must NOT infer: contain "hotfix"/"prod" as a substring only.
		{"origin/feature/CB-8888_workflow_status_hotfix", false},
		{"origin/fix/CB-9708-hotfix-temporal-duplicate", false},
		{"origin/cherry-pick/placeholder-purge-prod", false},
		{"origin/CB-9489-fix-e2e-prod-5xx", false},
		{"origin/cb-14147-prod-ga-id", false},
		// Must infer: genuine hotfix/release/production bases.
		{"origin/hotfix/2026-03-13", true},
		{"origin/hotfix/2026-07-27-mention-citation-defs", true},
		{"origin/hotFix/regressionOnPrePublishEdit", true}, // case-insensitive
		{"origin/release/1.2.3", true},
		{"origin/production", true},
		{"production", true},
		// Defaults / non-hotfix.
		{DefaultBaseBranch, false},
		{"origin/main", false},
		{"cb-15329/propagation-in-publish", false},
	}
	for _, tt := range tests {
		if got := IsHotfixBase(tt.base); got != tt.want {
			t.Errorf("IsHotfixBase(%q) = %v, want %v", tt.base, got, tt.want)
		}
	}
}

func TestInferHotfixBranch(t *testing.T) {
	tests := []struct {
		base, branch string
		want         string
		wantApplied  bool
	}{
		{"origin/hotfix/2026-03-13", "gig-ab12-fix", "hotfix/gig-ab12-fix", true},
		{"origin/production", "gig-ab12-fix", "hotfix/gig-ab12-fix", true},
		// Non-hotfix base: unchanged.
		{"origin/main", "gig-ab12-fix", "gig-ab12-fix", false},
		// Already prefixed: no double-prefix.
		{"origin/production", "hotfix/gig-ab12-fix", "hotfix/gig-ab12-fix", false},
	}
	for _, tt := range tests {
		got, applied := InferHotfixBranch(tt.base, tt.branch)
		if got != tt.want || applied != tt.wantApplied {
			t.Errorf("InferHotfixBranch(%q, %q) = (%q, %v), want (%q, %v)",
				tt.base, tt.branch, got, applied, tt.want, tt.wantApplied)
		}
	}
}
