// types.go — v1 memory subsystem types, scopes, buckets, and path resolution.
// Coexists with the legacy /learn API in memory.go (phased out by EPIC gig-1d33).
package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// MemoryType is the worker-facing type of a memory entry.
// Mirrors Claude Code's 3-field schema (name, description, type).
type MemoryType string

const (
	TypeUser      MemoryType = "user"
	TypeFeedback  MemoryType = "feedback"
	TypeProject   MemoryType = "project"
	TypeReference MemoryType = "reference"
)

// Valid reports whether t is one of the four canonical memory types.
func (t MemoryType) Valid() bool {
	switch t {
	case TypeUser, TypeFeedback, TypeProject, TypeReference:
		return true
	}
	return false
}

// ParseMemoryType validates and returns a MemoryType, or an error.
func ParseMemoryType(s string) (MemoryType, error) {
	t := MemoryType(s)
	if !t.Valid() {
		return "", fmt.Errorf("invalid memory type %q (want user|feedback|project|reference)", s)
	}
	return t, nil
}

// ScopeKind identifies which canonical sub-tree a memory entry lives under.
type ScopeKind string

const (
	ScopePersona      ScopeKind = "persona"
	ScopeRepo         ScopeKind = "repo"
	ScopeProject      ScopeKind = "project"
	ScopeOrchestrator ScopeKind = "orchestrator"
)

// Valid reports whether s is one of the four canonical scopes.
func (s ScopeKind) Valid() bool {
	switch s {
	case ScopePersona, ScopeRepo, ScopeProject, ScopeOrchestrator:
		return true
	}
	return false
}

// Bucket is a CoALA-aligned partition within a scope.
type Bucket string

const (
	BucketCore       Bucket = "core"
	BucketProcedural Bucket = "procedural"
	BucketSemantic   Bucket = "semantic"
	BucketEpisodic   Bucket = "episodic"
)

// Buckets enumerates all canonical buckets in a fixed order.
var Buckets = []Bucket{BucketCore, BucketProcedural, BucketSemantic, BucketEpisodic}

// Valid reports whether b is one of the four canonical buckets.
func (b Bucket) Valid() bool {
	switch b {
	case BucketCore, BucketProcedural, BucketSemantic, BucketEpisodic:
		return true
	}
	return false
}

// ---- Roots ----

// MemoryRoot is JEFF_HOME/memory — canonical store, single-writer (marlowe).
func MemoryRoot(jeffHome string) string { return filepath.Join(jeffHome, "memory") }

// ProposalsRoot is JEFF_HOME/proposals — workers write here via `jeff memory propose`.
func ProposalsRoot(jeffHome string) string { return filepath.Join(jeffHome, "proposals") }

// QueueRoot is JEFF_HOME/queue — SessionEnd hook drops entries here.
func QueueRoot(jeffHome string) string { return filepath.Join(jeffHome, "queue") }

// QueueSessionsRoot is JEFF_HOME/queue/sessions — per-session JSON entries.
func QueueSessionsRoot(jeffHome string) string {
	return filepath.Join(QueueRoot(jeffHome), "sessions")
}

// TranscriptsRoot is JEFF_HOME/transcripts — copies of session transcripts.
func TranscriptsRoot(jeffHome string) string { return filepath.Join(jeffHome, "transcripts") }

// ArchiveRoot is JEFF_HOME/archive — processed proposals + queue entries.
func ArchiveRoot(jeffHome string) string { return filepath.Join(jeffHome, "archive") }

// ---- Scope paths ----

// PersonaScopePath returns the canonical scope dir for a persona.
func PersonaScopePath(jeffHome, persona string) string {
	return filepath.Join(MemoryRoot(jeffHome), "personas", persona)
}

// RepoScopePath returns the canonical scope dir for a repo.
func RepoScopePath(jeffHome, repo string) string {
	return filepath.Join(MemoryRoot(jeffHome), "repos", repo)
}

// ProjectScopePath returns the canonical scope dir for a project.
func ProjectScopePath(jeffHome, project string) string {
	return filepath.Join(MemoryRoot(jeffHome), "projects", project)
}

// OrchestratorScopePath returns marlowe's own canonical scope dir.
func OrchestratorScopePath(jeffHome string) string {
	return filepath.Join(MemoryRoot(jeffHome), "orchestrator")
}

// BucketPath returns the path to a bucket within a scope.
//
// For BucketCore the on-disk artifact is the file <scope>/core.md (callers add
// the .md extension). For procedural/semantic/episodic the returned path is a
// directory containing entry files.
func BucketPath(scopePath string, bucket Bucket) string {
	return filepath.Join(scopePath, string(bucket))
}

// ProposalsTaskPath returns proposals/<persona>/<task>.
func ProposalsTaskPath(jeffHome, persona, task string) string {
	return filepath.Join(ProposalsRoot(jeffHome), persona, task)
}

// ---- Layout ----

// EnsureLayout creates every directory the v1 memory subsystem expects under
// jeffHome. Idempotent: calling repeatedly is a no-op after the first success.
func EnsureLayout(jeffHome string) error {
	if jeffHome == "" {
		return fmt.Errorf("EnsureLayout: empty jeffHome")
	}
	dirs := []string{
		MemoryRoot(jeffHome),
		filepath.Join(MemoryRoot(jeffHome), "personas"),
		filepath.Join(MemoryRoot(jeffHome), "repos"),
		filepath.Join(MemoryRoot(jeffHome), "projects"),
		OrchestratorScopePath(jeffHome),
		ProposalsRoot(jeffHome),
		QueueRoot(jeffHome),
		QueueSessionsRoot(jeffHome),
		TranscriptsRoot(jeffHome),
		ArchiveRoot(jeffHome),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("ensure %s: %w", d, err)
		}
	}
	return nil
}
