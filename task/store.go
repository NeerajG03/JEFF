// Package task owns the pickup/teardown lifecycle for a gig task: claiming it,
// building its workspace and worktrees, wiring hooks/skills/memory, and the
// mirror-image teardown. It was extracted from cmd/jeff so the orchestration is
// testable and reusable — remote workers (EPIC Jeff-Anywhere, Phase A) run the
// same Pickup against an HTTP-backed Store instead of a local *gig.Store.
//
// Import rule: only cmd/jeff may import this package. Nothing under workspace/,
// memory/, hooks/, skill/, or persona/ may import it, or the extraction would
// re-introduce the cycle it was meant to break.
package task

import "github.com/NeerajG03/gig"

// Store is the slice of gig the pickup/teardown lifecycle needs. *gig.Store
// satisfies it; remote workers pass an HTTP-backed implementation that talks to
// the hub. Defining Pickup/Teardown against this interface (rather than
// *gig.Store) is the "Amendment to PLAN-Pickup-Rollback" from
// roadmaps/EPIC-Jeff-Anywhere.md — keep the method set in sync with it.
//
// EnsureAttrs is deliberately absent: it registers attribute definitions and is
// a *gig.Store-only concern, so callers invoke it before Pickup.
type Store interface {
	// Prefix is the task-ID prefix this store generates IDs with (gig default:
	// "gig"). Anything recovering a task ID from a path or slug must match
	// against it rather than a hardcoded literal — a custom `gig config set
	// prefix` otherwise breaks task resolution (#97).
	Prefix() string
	Get(id string) (*gig.Task, error)
	GetFull(id string) (*gig.Task, error)
	Claim(id, assignee string) (*gig.ClaimResult, error)
	UpdateStatus(id string, st gig.Status, actor string) error
	Update(id string, p gig.UpdateParams, actor string) (*gig.Task, error)
	SetAttr(taskID, key, value string) error
	GetAttr(taskID, key string) (*gig.Attribute, error)
	AddCheckpoint(taskID, author string, p gig.CheckpointParams) (*gig.Checkpoint, error)
	LatestCheckpoint(taskID string) (*gig.Checkpoint, error)
	CloseTask(id, reason, actor string) error
	AddComment(taskID, author, content string) (*gig.Comment, error)
}
