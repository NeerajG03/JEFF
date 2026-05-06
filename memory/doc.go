// doc.go — content for `jeff memory doc`.
package memory

// Doc is the brief memory-system explainer printed by `jeff memory doc`.
const Doc = `# JEFF memory

Workers propose; only marlowe (the curator) writes canonical memory.
Native CLI memory is suppressed — JEFF owns the surface.

## Flow

1. Worker session starts. Task CLAUDE.md tells the agent how to capture.
2. Worker runs ` + "`jeff memory propose --name <slug> --type <t> --description <d> --body <b>`" + `
   when it observes a learning. Proposal lands in proposals/<persona>/<task>/.
3. SessionEnd drops a queue entry; the transcript is copied for marlowe.
4. ` + "`jeff memory curate`" + ` runs marlowe — dedupes, soft-invalidates conflicts,
   writes enriched canonical entries to memory/<scope>/<bucket>/.

## Schema (3 fields)

  ---
  name: <slug>            kebab-case, ≤64 chars
  description: <one-line>
  type: user | feedback | project | reference
  ---
  body…

Don't worry about scope or destination — marlowe decides.

## Types

  user        facts about the user (preferences, role)
  feedback    corrections or guidance the user gave
  project     ongoing work, decisions, stakeholders
  reference   pointers to external systems (Slack, Linear, dashboards)

## Permission model

Only marlowe can write canonical (` + "`JEFF_MEMORY_CAN_ADD=1`" + ` is set in marlowe
sessions only). ` + "`jeff memory add`" + ` refuses without it. Workers use ` + "`propose`" + `.

## Commands

  jeff memory propose      capture (any persona)
  jeff memory list         see canonical entries
  jeff memory show <name>  print one entry
  jeff memory status       queue depth, counts, last curate
  jeff memory diff <name>  bi-temporal supersede chain
  jeff memory curate       run marlowe
  jeff memory migrate      move legacy layout into v1 tree
  jeff memory disable      toggle in jeff.json

## Disabling

  export JEFF_MEMORY_DISABLE=1     this shell only
  jeff memory disable --confirm    persistent (jeff.json)

Canonical memory is never deleted on disable; superseded entries get
` + "`valid_to`" + ` — full audit trail preserved.
`
