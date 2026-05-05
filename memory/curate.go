// curate.go — stub: Worker C fills in.
// See exports/memory-research/specs/C-curate.md
//
// Owner C implements the marlowe curation loop: read queue + proposals,
// invoke the marlowe persona with .skills/curation/SKILL.md, write canonical
// entries (the only writer to JEFF_HOME/memory/**), soft-invalidate
// supersedes, and archive processed inputs.
package memory

// TODO(C): implement curation loop, supersede semantics, archive sweep.
