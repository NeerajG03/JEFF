---
description: Explicitly save a memory (escape hatch when you want to be sure something is captured)
---

Ask the user what they want to remember, then run:

  jeff memory propose --name <slug> \
                      --type <user|feedback|project|reference> \
                      --description "<one-line>" \
                      --body "<details>"

Confirm to the user: "Proposed: <path>. marlowe will consolidate this on next curate."
