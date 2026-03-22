# Skill Management

JEFF manages agent skills — SKILL.md files that provide reusable instructions and context. Skills are stored centrally and auto-injected into task workspaces via symlinks.

## Quick Reference

```bash
jeff skill list                              # list all skills
jeff skill list --persona jock               # filter by persona
jeff skill show <name>                       # show details + SKILL.md
jeff skill add <path>                        # copy skill into .skills/
jeff skill add <path> --external             # register without copying
jeff skill remove <name>                     # unregister
jeff skill remove <name> --delete            # unregister + delete files
jeff skill tag <name> --persona jock,scout   # set persona filter
jeff skill tag <name> --type bug,feature     # set task type filter
jeff skill tag <name> --tag deploy,ci        # set tag filter
jeff skill inject <name>                     # symlink into current task
jeff skill eject <name>                      # remove from current task
```

## How Skills Are Stored

```
JEFF_HOME/
├── .skills/
│   ├── skills.json          # registry (JSON with schema)
│   ├── deploy/
│   │   └── SKILL.md
│   └── review-pr/
│       ├── SKILL.md
│       └── examples/
```

`skills.json` maps skill names to their location and injection criteria:

```json
{
  "$schema": "https://raw.githubusercontent.com/NeerajG03/JEFF/main/schemas/skills.json",
  "skills": {
    "deploy": {
      "location": "/path/to/.skills/deploy",
      "tags": ["deploy"],
      "personas": ["jock"],
      "gig_type": ["chore", "feature"]
    }
  }
}
```

Skills can live anywhere — `location` points to the directory containing SKILL.md. Use `--external` when adding to keep skills at their original path.

## Auto-Injection on Pickup

When `jeff pickup <id> --persona <name>` runs, JEFF matches skills against:

- **personas**: if the pickup persona matches any listed persona
- **gig_type**: if the task type matches any listed type
- **tags**: if any tag intersects with the task's gig labels

**Any single dimension match is enough to inject.** Empty dimensions are ignored. If all dimensions are empty, the skill is never auto-injected (manual only).

Matched skills are symlinked into the task's skills directory for automatic discovery.

## Creating a Skill

1. Create a directory with a `SKILL.md` file:

```bash
mkdir my-skill
cat > my-skill/SKILL.md << 'EOF'
---
name: my-skill
description: What this skill does
---

Skill instructions here.
EOF
```

2. Register it:

```bash
jeff skill add ./my-skill
jeff skill tag my-skill --persona jock --type feature
```

The skill will now auto-inject for any task picked up by the jock persona or any feature-type task.

## Manual Injection

From inside a task workspace:

```bash
jeff skill inject deploy     # adds skill to current task
jeff skill eject deploy      # removes it
```

This creates/removes a symlink in the task's skills directory.
