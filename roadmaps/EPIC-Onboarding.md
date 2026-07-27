# EPIC-Onboarding: make JEFF usable by someone who has never seen it

> **Type:** Epic (5 phases) · **Leverage:** Adoption — every other feature is gated behind a first-run that currently doesn't finish · **Effort:** Medium; Phase 0 is docs-only and ships standalone
>
> Goal, stated as the test we must pass: **a developer who has never used JEFF or gig
> can go from "never heard of it" to "one task shipped" in under ten minutes, without
> reading source code, and without a human helping them.** Today they cannot — not
> because the machinery is missing, but because the entry path is undocumented,
> non-interactive, and in three places documents commands that do not exist.

---

## Part 1 — What is actually broken (audit, with evidence)

Every row below was verified against the tree at `b5c0e52`. This is the work list; the
phases that follow are just an ordering of it.

### A. Documentation says things that are false or missing

| # | Gap | Evidence | Impact |
|---|---|---|---|
| A1 | **gig is never mentioned in Install.** The README's own flow diagram starts at `gig create`, but Requirements sat at line 243 — below 240 lines of crew docs. | `README.md` (pre-change) §Install vs §Requirements | A new user runs `jeff pickup` against a task they have no way to create. Hard stop on line 1 of the quickstart. |
| A2 | **`jeff persona set-model` is documented but does not exist.** | `cmd/jeff/persona_cmd.go:19-24` wires `list, show, add, remove, tag` only; `grep -rn "set-model" --include=*.go` returns nothing | Command-not-found on a documented command destroys trust in the rest of the table. |
| A3 | **`jeff orchestrator init` is absent from the command table**, yet `jeff crew start` hard-fails without it. | `cmd/jeff/crew_cmd.go:137-150` returns *"no orchestrator identity found"*; `orchestrator_cmd.go:44` defines `init` | Crew mode — the headline feature — is unreachable by following the README. |
| A4 | **`jeff memory` is absent from the command table** though it is wired and has 8 subcommands. | `cmd/jeff/main.go:61`; `cmd/jeff/memory/memory_cmd.go:21-31` | A whole subsystem is invisible. |
| A5 | **Quickstart uses a fabricated task ID** (`gig-ab12`). | `README.md` (pre-change) §Quick Start | Copy-paste fails; user cannot tell whether JEFF is broken or they are. |
| A6 | **`skip_permissions: true` default is disclosed on line 239**, below everything. | `config.go:41-46` (nil → true) | A security-relevant default nobody reads. |
| A7 | `jeff repo post-setup`, `jeff persona add/remove/tag`, `jeff skill doc`, `jeff doctor --json` all exist and were undocumented. | `repo_cmd.go:144`, `persona_cmd.go:19-24`, `skill_cmd.go:35`, `doctor_cmd.go:149` | Users re-implement things JEFF already does. |

### B. `jeff doctor` does not answer "am I ready?"

| # | Gap | Evidence | Impact |
|---|---|---|---|
| B1 | **Does not check gig at all** — the one hard requirement. | `getDoctorDeps()`, `cmd/jeff/doctor_cmd.go:38-111`: tmux, git, terminal-notifier, gh, jq + agent providers. No gig. | `jeff doctor` says ✓ on a machine where nothing can work. |
| B2 | **Install hints are macOS/Homebrew-only.** | same function — every `InstallCmd` is `brew install …`, plus three `npm install -g` | Linux users are handed a command they cannot run. `terminal-notifier` is offered on Linux, where it does not exist. |
| B3 | **Checks binaries, not readiness.** No check for: JEFF initialized, gig store reachable, ≥1 repo registered, configured agent actually installed, `gh` authenticated. | whole file | The most common failure — `cfg.Agent = claude` (default) while only `opencode` is installed — is invisible until launch. |
| B4 | **tmux is `Required: true`** but is only needed for crew mode. | `doctor_cmd.go:41-47` | Solo users get a red ✗ and a non-zero exit for a dependency they don't need. |

### C. `jeff init` makes decisions for the user, silently

| # | Gap | Evidence | Impact |
|---|---|---|---|
| C1 | **Fully non-interactive.** Never asks agent, IDE, repos, or hooks. | `runInit()`, `cmd/jeff/init_cmd.go:37-141` | Everyone lands on `agent: claude`, `ide: unset` regardless of what they use. |
| C2 | **Default config hardcodes `claude`** with no detection of what's installed. | `config.go:50-56` | See B3. Silent misconfiguration is the single most likely first-run failure. |
| C3 | **Closing output is a directory listing, not next steps.** | `init_cmd.go:132-139` | The user is told what folders exist, not what to type next. |
| C4 | **Does not run or reference `jeff doctor`.** | same | Missing deps surface later as cryptic runtime errors. |
| C5 | **Warnings are printed and swallowed** for persona seeding, skill seeding, and skills aliasing. | `init_cmd.go:99-115` | A half-initialized home reports success. |

### D. The gig cliff

| # | Gap | Evidence | Impact |
|---|---|---|---|
| D1 | **JEFF cannot create a task.** No `jeff task new`. Users must install and learn a second CLI before JEFF does anything. | `main.go:56-79` command list | Doubles the tools-to-learn count at the worst possible moment. |
| D2 | **`jeff.json`'s `gig_home` never reaches the store.** `openGigStore()` calls `gig.LoadConfig("")` and ignores `cfg.GigHome`; only hooks receive it. | `cmd/jeff/gig.go:10-22` vs `hooks/hook.go:44`, `task/pickup.go:217`, `cmd/jeff/hook_sync.go:55` | Setting `gig_home` makes hooks and the CLI read *different stores*. Silent split-brain. |
| D3 | **No `jeff config gig-home` setter** though every other config field has one. | `cmd/jeff/config_cmd.go:34-39` | Users hand-edit JSON for a field that then doesn't take effect (D2). |
| D4 | **gig's SDK falls back to defaults when uninitialized**, so JEFF silently creates a store with prefix `gig`. | gig `config.go:71-105` — `os.IsNotExist` → `DefaultConfig()` | User runs `gig init --prefix myapp` later and now has two ID namespaces. |

### E. Crew mode's first run

| # | Gap | Evidence | Impact |
|---|---|---|---|
| E1 | The `orchestrator init` → `orchestrator start` → `crew start` ordering is load-bearing and undocumented. | `crew_cmd.go:137-150` | See A3. |
| E2 | Orchestrator identity is **per-directory**, which is never stated anywhere. | `crew_cmd.go:88-100, 262-275` | Users run `orchestrator init` in the wrong place and get the same error again. |

### F. Skills / personas / memory have no on-ramp

| # | Gap | Evidence | Impact |
|---|---|---|---|
| F1 | `jeff skill add ./my-skill` is the documented entry point, but there is no skill to add and no catalogue. | `README.md` §Skills (pre-change) | Feature reads as aspirational. |
| F2 | Only `crew-orchestrator` is seeded, and it is untagged (no persona injection). | `skill/templates.go:23-48` | Nothing is injected on a fresh install, so skills look broken. |
| F3 | Memory's propose → curate loop, and the fact that promotion is human-triggered, is documented in two sentences. | `README.md` §Agent Memory (pre-change); `cmd/jeff/memory/memory_cmd.go:13-18` | Users assume memory is automatic, then conclude it doesn't work. |

---

## Part 2 — The plan

Sequencing principle: **fix truth before adding machinery.** Phase 0 costs no Go code
and removes the majority of first-run failures. Everything after it reduces the number
of things the user has to know.

```
Phase 0 ─ docs are true + agent-driven setup   (docs only, ships now)
   │
   ├─► Phase 1 ─ `jeff doctor` answers "am I ready?"   (the readiness oracle)
   │        │
   │        └─► Phase 2 ─ `jeff init` becomes a wizard   (consumes Phase 1)
   │
   ├─► Phase 3 ─ remove the gig cliff (`jeff task`, gig_home correctness)
   │
   ├─► Phase 4 ─ crew mode's first run
   │
   └─► Phase 5 ─ skills/personas/memory on-ramp
```

Phases 1 and 3 are independent and can run in parallel. Phase 2 depends on Phase 1
(the wizard's first screen *is* doctor's structured output). Phases 4 and 5 are small
and can land any time after 0.

---

### Phase 0 — Make the docs true, and let an agent do the setup ✅ *this change*

**Closes:** A1–A7, and mitigates B1–B2, C1–C4, D1, D4, E1–E2, F1–F3 by documenting them.

No Go changes. Three deliverables:

1. **`README.md` rewritten** around the actual first run:
   - Two labelled entry paths in the first ten lines: *let an agent set it up* / *5-minute manual quickstart*.
   - **Prerequisites table before Install**, with gig listed first and a *why* column.
   - Install shows `gig` and `jeff` together, all three channels, plus the PATH fix for `go install`.
   - Quickstart is six commands that work as written, using `gig init --prefix myapp` → `gig create` → the printed ID.
   - `skip_permissions` and hooks-on-by-default called out immediately after the quickstart, not on line 239.
   - Command table corrected: `persona set-model` removed, `orchestrator init` added and annotated as required-first, `memory` and `repo post-setup` added.
   - Crew section leads with `jeff orchestrator init` and says identity is per-directory.
   - A `## Documentation` index so the other five docs are discoverable.

2. **`docs/agent-setup.md`** — an agent-executable runbook, fetchable raw:
   `https://raw.githubusercontent.com/NeerajG03/JEFF/main/docs/agent-setup.md`

   Design constraints that make it work as a tool rather than prose:
   - **Ground rules first** — never replace a live `jeff` binary (crew workers die), never overwrite an existing home, never invent task IDs, don't put secrets in `jeff.json`.
   - **Step 0 is a read-only survey** whose output the later steps branch on, so the runbook is safe to start on a machine that is already half set up.
   - **Explicit `ASK` blocks** with stated defaults — the human answers 5 questions total (prefix, agent, IDE, repos, crew yes/no).
   - **A verification command after every step**, with the expected output named.
   - **It compensates for today's gaps**: installs gig itself (B1), carries a Linux/Fedora/Arch translation table for doctor's Homebrew hints (B2), sets `jeff config agent` explicitly because the default is `claude` regardless (C2), runs `jeff orchestrator init` in the right directory (E1–E2), and warns about `gig_home` (D2).
   - **Step 10 takes one real task through `pickup → checkpoint → ship --dry-run → done`** — the runbook is not allowed to declare success without an end-to-end pass.
   - **A fixed-shape final report** including a mandatory *"Not done / needs your attention"* section, so a partial setup names its own gaps.
   - A symptom→cause→fix troubleshooting table covering the nine errors a first-timer actually hits.

3. **The paste-me prompt**, in the README, pointing at the raw URL. It stays five lines
   because all the depth lives in the fetched runbook — that is the point of splitting
   them.

**Acceptance:** on a clean container with neither tool installed, pasting the README
prompt into Claude Code, opencode, and Gemini CLI each ends with a shipped-and-closed
task and a truthful final report. Every command appearing in README or the runbook
resolves (`jeff <cmd> --help` exits 0).

**Note:** the raw URL 404s until this lands on `main`. Do not merge the README without
the runbook in the same commit.

---

### Phase 1 — `jeff doctor` becomes the readiness oracle

**Closes:** B1–B4. **Enables:** Phase 2, and lets the Phase 0 runbook stop compensating.

Today doctor answers "which binaries exist". It must answer "can I run `jeff pickup`
right now, and if not, what do I type".

**1.1 — Add gig as a required dependency.** New entry in `getDoctorDeps()`
(`doctor_cmd.go:38`): binary `gig`, `--version`, install `brew install neerajg03/tap/gig`
/ `go install github.com/NeerajG03/gig/cmd/gig@latest`.

**1.2 — Platform-aware install hints.** Replace the `InstallCmd string` field with

```go
// InstallHint returns the platform-appropriate install command, or "" when the
// dependency does not apply to this platform (e.g. terminal-notifier off macOS).
type dep struct {
    // ...
    Install map[string]string // "darwin" | "linux" | "windows" | "" (fallback)
    OnlyOn  []string          // empty = all platforms
}
```

Select on `runtime.GOOS`. Linux entries use a distro probe (`/etc/os-release` `ID`/
`ID_LIKE`) with an apt/dnf/pacman table and a documentation URL as fallback. Drop
`terminal-notifier` entirely off darwin via `OnlyOn`.

**1.3 — Scope tmux to crew mode.** Change tmux to `Required: false` with a new
`RequiredFor: "crew"` field, rendered as `⚠ needed for crew mode` rather than `✗`, and
excluded from the required-failed exit code. `jeff crew start`/`orchestrator start`
already fail loudly on their own; doctor should not block solo users. (Note:
`doctor_cmd_test.go` asserts on the current dep set — update alongside.)

**1.4 — Add environment checks, not just binaries.** A second section, `ENVIRONMENT`,
each row `ok | warn | fail` with a fix string:

| Check | Fail condition | Fix shown |
|---|---|---|
| jeff initialized | `ResolveHome()` errs, or no `jeff.json` at the resolved home | `jeff init` |
| gig store reachable | `openGigStore()` errs | `gig init --prefix <name>` |
| gig store initialized | gig config file absent (store would use fallback defaults — D4) | `gig init --prefix <name>` |
| configured agent installed | `exec.LookPath(string(cfg.Agent))` fails | `jeff config agent <installed>` — name the ones that *are* installed |
| ≥1 repo registered | `len(ListRepos(cfg)) == 0` | `jeff repo add <url>` |
| gh authenticated | `gh auth status` non-zero (warn, not fail) | `gh auth login` |
| hooks in sync | any generated script's version marker ≠ `hooks.ScriptVersion` (reuse `TaskHooksStale`, `hooks/hook.go:82`) | `jeff config hooks sync` |

Environment checks need `cfg`, which `PersistentPreRunE` skips only for `jeff init`
(`main.go:25`) — but doctor must survive an *uninitialized* home, which today is a hard
error before `RunE` runs. So: add `jeff doctor` to the skip-list and load config
defensively inside the command, treating "not initialized" as the first finding rather
than a crash. This is the one non-obvious refactor in the phase.

**1.5 — Stable `--json` contract.** The Phase 0 runbook and any wizard consume this, so
version it:

```jsonc
{
  "version": 1,
  "ok": false,                       // no required dep and no environment check failing
  "platform": {"os": "linux", "distro": "debian"},
  "deps": [{"name":"gig","status":"missing","required":true,"install":"brew install …"}],
  "environment": [{"name":"jeff_initialized","status":"fail","fix":"jeff init"}],
  "next": ["jeff init", "gig init --prefix myapp"]   // ordered, copy-pasteable
}
```

`next` is the payload the wizard and the agent runbook both act on.

**1.6 — Exit codes.** `0` all good · `1` a required dep or environment check failed ·
`2` warnings only. Today any required failure returns a generic error (`doctor_cmd.go:136`);
scripts cannot distinguish "broken" from "suboptimal".

**1.7 — `jeff doctor --fix`.** Runs only the safe, JEFF-side repairs — never a package
install: `jeff init --update`, hook sync, `EnsureAttrs` (`attrs.go`). Prints each action
before running it, and re-runs the checks afterwards. Anything requiring a package
manager or a credential is printed, not executed.

**Acceptance:** on a container with nothing installed, `jeff doctor --json | jq -r
'.next[]'` emits an ordered command list that, executed verbatim, ends with
`jeff doctor` exiting 0. `terminal-notifier` never appears on Linux. tmux missing does
not produce exit 1. Unit tests cover: platform hint selection per GOOS/distro, each
environment check in both states via `t.TempDir()` homes, the JSON shape, and all three
exit codes.

---

### Phase 2 — `jeff init` becomes a wizard (with a scriptable escape hatch)

**Closes:** C1–C5. **Depends on:** Phase 1 (uses `doctor --json`).

The rule: **interactive when a human is watching, silent and flag-driven when not.**
Agents and CI must never be prompted.

**2.1 — Mode selection.** Interactive iff `term.IsTerminal(os.Stdin.Fd())` and no
`--yes`. `--yes` (alias `--non-interactive`) takes every default without asking; every
prompt also has a flag (`--agent`, `--ide`, `--repo` repeatable, `--gig-prefix`,
`--no-repos`) so the whole wizard is bypassable. `CI=true` in the environment implies
`--yes`.

**2.2 — Wizard flow.** Each step idempotent and re-runnable; `^C` at any point leaves a
valid (if partial) home, and re-running resumes.

```
1. Readiness      run the Phase-1 checks. Required missing → print `next`, offer to
                  continue anyway (default: no). Never silently proceed past a
                  missing agent CLI.
2. Home           confirm the resolved home. Already initialized → switch to update
                  mode and say so (never clobber — C-series ground rule).
3. gig            store not initialized → offer `gig init --prefix <suggested>`,
                  suggestion derived from the cwd's git remote name.
4. Agent          detect installed CLIs via LookPath; default to the single one
                  found, else prompt. THIS is the fix for C2 — never write
                  `claude` unexamined.
5. IDE            detect `code`/`cursor`/`windsurf`/`nvim` on PATH, default to the
                  first found.
6. Repos          loop: "clone URL (blank to finish)". Per repo, offer a short name
                  and a description. Clone failures are reported and retried, not
                  fatal — a home with zero repos is still valid.
7. Permissions    state the skip_permissions default in one sentence and ask
                  keep/enable (A6 fix, at the moment it is decidable).
8. Crew           "set up crew mode?" → if yes, check tmux and explain that
                  `jeff orchestrator init` is per-directory (E-series).
9. Summary        the settings, then the next-steps ladder (2.3).
```

**2.3 — Replace the directory listing (C3).** Final output becomes:

```
JEFF is ready at /home/you/.jeff

  agent  claude        ide  cursor        repos  backend, frontend

Next:
  gig create "your first task"        create work
  jeff pickup <id> --repos backend    claim it and launch claude
  jeff status                         see what's in flight
  jeff doctor                         re-check readiness any time
```

Keep the directory tour behind `jeff init --verbose`; it is reference material, not a
next step.

**2.4 — Stop swallowing seed failures (C5).** Persona seeding, skill seeding, and the
skills-alias calls at `init_cmd.go:99-115` currently warn-and-continue. Collect them
and report `Initialized with 2 warnings — run jeff doctor` with a non-zero-but-distinct
exit (`2`), so "half-initialized" is not indistinguishable from success.

**2.5 — Dependency.** One new import: `golang.org/x/term` for TTY detection (already an
indirect dep via bubbletea — verify with `go mod why` before adding it directly).
Prompting is hand-rolled `bufio.Scanner` over stdin; do **not** pull in a prompt
library for nine questions, and do not use bubbletea here — a full-screen TUI over a
half-configured home is a debugging nightmare.

**Acceptance:** `jeff init --yes` on a clean container is byte-comparable to today's
non-interactive behavior plus the new summary (existing `init_test.go` assertions hold).
A scripted-stdin test drives the full interactive path. `jeff init` twice in a row is
safe and the second run reports update-mode. Interactive mode never triggers under
`CI=true` or a non-TTY stdin.

---

### Phase 3 — Remove the gig cliff

**Closes:** D1–D4.

**3.1 — `jeff task` (D1).** Thin wrappers over the gig SDK — no new state, matching the
SDK-first convention:

```
jeff task new "<title>" [--type] [--priority] [--parent] [--json]   → prints the id
jeff task list [--ready] [--mine] [--json]
jeff task show <id> [--json]
```

`jeff task new` must print the ID on its own line so the docs' `$(jeff task new …)`
idiom works. This is what lets the quickstart become *one* tool, and lets `jeff init`'s
next-steps ladder say `jeff task new` instead of teaching a second CLI. gig stays fully
supported and documented for everything else (deps, hierarchy, `gig ui`, search) — we
are covering the 90% path, not wrapping gig.

**3.2 — Fix `gig_home` (D2).** `openGigStore()` (`cmd/jeff/gig.go:10`) must resolve the
store the same way hooks do. Precedence, documented in one place:
`GIG_HOME` env → `jeff.json` `gig_home` → gig's own default. Since `openGigStore()`
currently takes no config, thread `cfg` in (it is a package-level var in `cmd/jeff`) and
pass `cfg.GigHome` to `gig.LoadConfig`. Add a test asserting hooks and the CLI resolve
to the same path for all three precedence cases — this is a silent-data-split bug, so
the test is the deliverable as much as the fix.

**3.3 — `jeff config gig-home [path]` (D3).** Get/set following the existing
`configEnumCmd` shape, validating that the path exists and warning if it contains no
gig store yet.

**3.4 — Detect the uninitialized-gig footgun (D4).** Covered by the Phase-1 environment
check; here just make sure `jeff task new` surfaces it rather than silently creating the
first task in a fallback store with prefix `gig`.

**Acceptance:** a user who has never run the gig CLI can do
`jeff init --yes && jeff task new "x" && jeff pickup <id> --repos r` end to end. Test
matrix for gig-home precedence. `jeff task new --json` shape is stable.

---

### Phase 4 — Crew mode's first run

**Closes:** E1–E2. Small phase; mostly ergonomics on top of correct machinery.

**4.1 — `jeff crew up [--name work]`** — one command that does `orchestrator init` (if
this directory has no identity) then `orchestrator start`, printing each step. The
existing error at `crew_cmd.go:137-150` is already excellent; this removes the need to
hit it.

**4.2 — Make identity-is-per-directory visible.** `jeff orchestrator init --help` and
`jeff orchestrator list` should both state the directory the identity is bound to.
`jeff orchestrator info` should show it. One sentence each; the concept only surprises
people because nothing prints it.

**4.3 — Doctor integration.** `jeff crew`/`jeff orchestrator` subcommands check tmux
presence and version (≥3.0) up front with the Phase-1 hint machinery, instead of failing
inside tmux calls.

**Acceptance:** `jeff crew up` from a fresh project directory produces a working
orchestrator session, and `jeff crew start` succeeds immediately afterwards. Existing
`orchestrator_init_test.go` and `crew_start_test.go` still pass.

---

### Phase 5 — On-ramp for skills, personas, memory

**Closes:** F1–F3. Lowest urgency, highest "now I get it" value.

**5.1 — Ship more than one skill (F2).** `crew-orchestrator` alone, untagged, means a
fresh install injects nothing. Seed 2–3 genuinely useful embedded skills tagged for the
personas that want them (candidates: `go-testing` → jenko, `pr-review` → hardy,
`root-cause` → schmidt) so `jeff pickup --persona hardy` visibly injects something on
day one. This is the difference between skills reading as a feature and reading as a
promise.

**5.2 — `jeff skill new <name>` (F1).** Scaffold a `SKILL.md` with frontmatter and a
worked example, then register it. `jeff skill doc` explains the format; scaffolding
removes the blank page.

**5.3 — Make memory legible (F3).** `jeff memory status` — proposals pending, last
curation, per-persona and per-repo counts — and one paragraph in `docs/usage.md` making
explicit that promotion is human-triggered via marlowe and nothing auto-promotes.

**5.4 — `jeff status` teaches when empty.** With no tasks and no repos, print the
getting-started ladder instead of an empty table. The empty state is a first-run
surface and currently wastes it.

**Acceptance:** fresh install → `jeff pickup <id> --persona hardy` shows ≥1 injected
skill. `jeff skill new x` produces a registered, valid skill. `jeff status` on an empty
home prints next steps.

---

## Non-goals

- **A GUI or web installer.** The paste-a-prompt path (Phase 0) is the "graphical"
  onboarding; it costs nothing to maintain.
- **Wrapping all of gig.** Phase 3.1 covers create/list/show. Dependencies, hierarchy,
  search, and `gig ui` stay gig's job, documented as such.
- **A bubbletea TUI for `init`.** Deliberate — see 2.5.
- **Changing the `skip_permissions` default.** Phase 0/2 make it *visible* and
  *decidable*. Flipping it is a separate call, tracked in `PLAN-Permission-Safety.md`.
- **Touching the remote/hub work.** `EPIC-Jeff-Anywhere.md` assumes a working local
  install; this epic is its prerequisite, not its competitor.

## Sequencing and effort

| Phase | Depends on | Size | Ships value alone |
|---|---|---|---|
| 0 — docs + agent runbook | — | S (docs only) | Yes — removes most first-run failures immediately |
| 1 — doctor as oracle | 0 | M | Yes |
| 2 — init wizard | 1 | M | Yes |
| 3 — gig cliff | — (parallel with 1) | M | Yes; 3.2 is a correctness fix worth landing regardless |
| 4 — crew first run | 0 | S | Yes |
| 5 — skills/personas/memory | 0 | S–M | Yes |

Recommended order: **0 → (1 ‖ 3) → 2 → 4 → 5**.

## The measurable definition of done

A container with only git and a shell, and a human who has never seen JEFF:

1. `jeff doctor` on the bare machine prints an ordered, correct, platform-appropriate
   list of what to do — and `--json`'s `next` array, executed verbatim, reaches exit 0.
2. The README quickstart works as written, top to bottom, no substitutions beyond a
   repo URL.
3. The paste-me prompt reaches a shipped-and-closed task with ≤5 questions asked.
4. `jeff init` never writes a config that contradicts the machine (no `agent: claude`
   without Claude Code).
5. Every command in the README exists; `jeff <cmd> --help` exits 0 for all of them.
   *Worth a CI check* — this epic exists partly because A2/A3/A4 were never caught.
6. A user can create their first task without installing the gig CLI (Phase 3).
