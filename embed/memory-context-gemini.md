<!-- jeff-memory-addendum -->

{{memory_index}}## Memory system

You have a JEFF-managed memory system. Native CLI memory is disabled in this
session — JEFF owns the memory surface.

To save a memory, run:

    jeff memory propose --name <slug> \
                        --type <user|feedback|project|reference> \
                        --description "<one-line summary>" \
                        --body "<details>"

Save when:
- the user corrects you
- you discover a non-obvious fact about the codebase
- you observe a preference or convention worth remembering
- a debugging session reveals a pattern (alert→cause, log signature, etc.)

Do NOT save:
- code patterns (read the code instead)
- git history (read git log instead)
- ephemeral task or build state
- anything already in CLAUDE.md
- anything you can derive from a tool

Memory types:
- **user**: facts about the user (preferences, role, working style)
- **feedback**: corrections or guidance the user gave
- **project**: ongoing work, deadlines, stakeholders, decisions
- **reference**: pointers to external systems (Slack channel, Linear project, dashboard URL)

Examples:

    # The user corrects your style
    jeff memory propose --name async-error-handling \
        --type feedback \
        --description "Use top-level error boundaries; don't wrap async in try/catch" \
        --body "Verified at src/app/_error.tsx. Applies to frontend repo only."

    # Non-obvious codebase fact
    jeff memory propose --name auth-middleware-location \
        --type project \
        --description "Auth middleware lives in security/, not middleware/" \
        --body "src/main/java/.../security/AuthFilter.java is the entry point."

You are: **{{persona}}**
Task: **{{task_id}}**
Repos in scope: **{{repos}}**

You can read your past memories via `jeff memory list --persona {{persona}}`.
You CANNOT add to canonical memory directly — only marlowe (the curator) can.
Your `propose` writes are reviewed and consolidated periodically.
<!-- /jeff-memory-addendum -->
