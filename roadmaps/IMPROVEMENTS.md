# JEFF Improvement Roadmap — Top 10, Ranked by Leverage

A holistic audit of the codebase (every package read end-to-end: root, `crew/`, `hooks/`, `memory/`, `skill/`, `persona/`, `workspace/`, `tui/`, `cmd/jeff/`, docs, schemas, CI — cross-checked against `docs/roadmap.md`). Each item below has a full execution plan in `roadmaps/PLAN-<Slug>.md` written so a less capable model can implement it without asking questions: goal, exact files, step order, edge cases found during exploration, and verifiable acceptance criteria.

## The ranking

| # | Plan | What it fixes | Leverage | Effort | Depends on |
|---|------|--------------|----------|--------|------------|
| 1 | [PLAN-Phase1-Attrs-Resume](PLAN-Phase1-Attrs-Resume.md) | The missing 30% of your own roadmap Phase 1: gig attrs (`persona`, `skills_loaded`, `memory_loaded`, `outcome`…) + checkpoint injection and persona agent/model resolution on `jeff work` | Very high | S-M | — |
| 2 | [PLAN-Crew-Reliability](PLAN-Crew-Reliability.md) | SQLite pragmas that silently don't apply (lost writes under concurrency), every crew message delivered twice, shared paste-buffer race, unversioned schema migrations | Very high | M | — |
| 3 | [PLAN-Memory-Correctness](PLAN-Memory-Correctness.md) | `jeff memory disable` is a no-op, failed curates destroy their inputs, one corrupt file breaks all reads, silent entry overwrites, "Last curate: never" forever, unbounded growth | Very high | M | — |
| 4 | [PLAN-Ship-Hardening](PLAN-Ship-Hardening.md) | `jeff ship` exits 0 on partial failure, records nothing in gig, leaks `.jeff-base` into PRs, ships past uncommitted work silently | High | S-M | — |
| 5 | [PLAN-Pickup-Rollback](PLAN-Pickup-Rollback.md) | A half-failed pickup wedges the task (claimed, no workspace, no un-claim); 185-line orchestration lives untested in the CLI layer; worktree scanning duplicated 4× | High | M-L | best after #1 |
| 6 | [PLAN-Hooks-Hardening](PLAN-Hooks-Hardening.md) | `jq` missing = all context injection silently vanishes; unescaped TaskID/pattern interpolation into bash; generated scripts drift forever (no version, no re-sync); JSONC settings break installs | High | M | — |
| 7 | [PLAN-Permission-Safety](PLAN-Permission-Safety.md) | `--dangerously-skip-permissions`/`--approval-mode=yolo` hardcoded into every launch with no off switch; `jeff done` silently `rm -rf`s dirty worktrees; no `git worktree prune` | High (trust) | S | — |
| 8 | [PLAN-Agent-Providers](PLAN-Agent-Providers.md) | Stringly-typed agent dispatch across ~15 files (adding an agent = ~15-file edit); `buildAgentCmd` builds broken commands for gemini/opencode; doctor doesn't know opencode | Strategic | M-L | after #6, #7 |
| 9 | [PLAN-Stats](PLAN-Stats.md) | Roadmap Phase 3: `jeff stats` over gig events+attrs — the feedback loop that tells you whether memory/personas actually work | Med-high | M | **needs #1** |
| 10 | [PLAN-Quality-Gates](PLAN-Quality-Gates.md) | Schemas reject valid configs (gemini/zed/memory missing; personas.json fails its own output); post-setup docs describe a contract the code doesn't implement; phantom `doug`, hidden `marlowe`; CI has no race/lint/macOS/push trigger | Medium | S-M | — |

## Do this first

**#1 (Phase1-Attrs-Resume).** It is the smallest of the top tier, it is literally your own roadmap's unfinished Phase 1, and three other things hang off it: `jeff stats` (#9) has no data layer without the attrs, the fragile `detectPersona` string-matching hack (used by `jeff done`'s curation and CLAUDE.md refresh) dies only when the persona is persisted, and `jeff work` currently resumes tasks with the wrong agent, no model, and no memory of prior progress — a bug your users hit every time they resume.

Then **#2 (Crew-Reliability)** — the per-connection PRAGMA bug means crew writes can silently vanish under exactly the concurrent load crew is built for (hooks + CLI + dashboard all hitting `jeff.db`), and the message double-delivery actively pollutes worker context today.

## Suggested execution waves

- **Wave 1 (independent, high value):** #1, #4, #7, #10 — all small-to-medium, no interdependencies, each lands standalone.
- **Wave 2 (correctness core):** #2, #3, #6.
- **Wave 3 (structure + payoff):** #5, then #8 (builds on #6/#7's touched files), then #9 (needs #1's attrs populated by real usage to be interesting).

Plans that touch the same files note the coordination explicitly (e.g. #1 and #5 both edit `pickupTask`; #5 says to land #1 first).

## Why these ten (and the evidence standard)

Every claim in the plans was verified against the working tree at commit `af1495e` — file:line citations throughout. The recurring patterns the audit surfaced:

1. **Shipped-but-inert features** — `jeff memory disable`, `.last-curated`, stall detection ("handled by jeff daemon" — no daemon exists), `jeff clean` (referenced in `jeff status` output, never implemented). These erode trust fastest because the tool *claims* the behavior.
2. **Silent failure as a default** — ship exits 0 on failure, crew `Refresh` discards write errors, hooks die quietly without `jq`, dirty worktrees are deleted without a word. For an autonomous-agent system, silent failure is the worst failure mode: nobody is watching.
3. **Two sources of truth** — provider registry vs string switches, schema vs structs, docs vs code, legacy memory API vs v1. Each pair has already diverged; #8 and #10 collapse them and add structural guards (consistency tests) so they can't quietly diverge again.
4. **The trust layer is aspirational** — the roadmap promises "the user controls the blast radius," but permissions are hardcoded off and teardown is destructive. #7 is small precisely because the mechanism just doesn't exist yet.

## Strategic track: JEFF Anywhere (hub + workers + chat)

Beyond the ten fixes, the big directional bet — *run JEFF as a worker on any machine, drive it from Slack or a chat UI, share memory/personas/skills across the fleet* — is designed in **[EPIC-Jeff-Anywhere.md](EPIC-Jeff-Anywhere.md)** (hub-and-spoke architecture, WS protocol, config-driven `jeff up` deployment, five phases that each ship standalone). It has a companion plan for the gig repo itself, **[PLAN-Gig-Upgrades.md](PLAN-Gig-Upgrades.md)** (CAS claim, pooled-PRAGMA fix, transactional events, ID-collision handling, event cursor — gig v0.7.0).

The epic is why several top-10 items are ranked where they are: **#5** provides `task.Pickup` as the worker's loop body (with a store-interface amendment specified in the epic), **#1** makes cross-worker resume possible (checkpoints are the only portable session state), **#2/#8/#7** are the crew/provider/permission groundwork the worker daemon sits on. Doing Wave 1–2 first means the epic starts on solid ground rather than distributing today's races.

## Honorable mentions (considered, ranked below the line)

Documented here so the findings aren't lost — each is real, none beat the ten above on value-per-effort:

- **TUI overhaul** — the dashboard has genuine bugs: pane capture is wiped by the 2s tick and blanks the Gigs tab (`tui/tui.go:117-119,580,602-606`), `View()` does O(N) SQLite reads per frame (`tui/sessions.go:47,93-112`), `height` is never used so long lists push the help bar off-screen, the `w` "start worker" key just prints a CLI hint, and the package has zero tests. Worth a plan when the daemon/approve flow gives the dashboard real actions to host.
- **CLI scripting UX** — `--json` on the read commands (`status`, `crew list`, `repo list`, `skill list`…), implement the advertised `jeff clean`, a `jeff tasks` listing (the query exists as shell-completion code), `--description` for `project init` (currently blocks on stdin), `SilenceUsage` set once on the root command.
- **Skill portability** — skill symlinks are absolute (task dirs break when JEFF_HOME moves — the gemini alias next to them is deliberately relative for exactly that reason, `embed/embed.go:32-33`); `copyDir` strips the executable bit and breaks symlinked files (`skill/skill.go:227-232`, same in `persona/registry.go`); `jeff init --update` force-overwrites user edits to embedded skills but never refreshes personas — pick one policy.
- **Crew liveness + daemon** — window-exists is the only health signal (a dead agent in a live pane is "running" forever; the stored PID is the shell's, not the agent's); `CheckStalls` has no scheduler and re-signals every run. This is really the roadmap's Phase 5 daemon — do it as a feature, not a fix.
- **Repo sync correctness** — `SyncRepo` hardcodes `main` (`repo.go:131,141,143`): repos defaulting to `master`/`develop` report "0 behind" and never sync; fix via `git symbolic-ref refs/remotes/origin/HEAD` + honor `base_branch`.
- **Atomic config writes** — `SaveConfig` (`config.go:196`) and friends are straight `os.WriteFile`; a crash mid-write corrupts `jeff.json` (the single source of repo registrations). Temp-file + rename helper, ~30 lines.
- **Legacy memory API retirement** — the dual-API state (`memory/CLAUDE.md`) double-injects memory at pickup (full legacy text + v1 index). Finishing worker E's migration (`gig-1d33.6`) halves the injected context.

---

*Generated from a full-codebase audit on 2026-07-07 (branch `claude/system-improvement-roadmap-0ix1yy`, base commit `af1495e`). Build, vet, and all tests were green at audit time.*
