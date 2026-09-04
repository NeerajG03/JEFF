# JEFF Agent Setup Runbook

> **You are an AI coding agent reading this file because a human asked you to set up JEFF for them.**
> Follow it top to bottom. Do not skip verification steps. Stop and report when something fails.
>
> Canonical URL (always fetch the version on `main`):
> `https://raw.githubusercontent.com/NeerajG03/JEFF/main/docs/agent-setup.md`

JEFF is an agent workspace manager: it gives AI agents structured task workspaces,
personas, skills, memory, and a multi-agent crew mode. It stores **no task state of
its own** — [gig](https://github.com/NeerajG03/gig) is the task database, and JEFF
drives it through the gig Go SDK.

By the end of this runbook the human will have: dependencies installed, a gig store,
an initialized JEFF home, a configured agent + IDE, at least one registered repo, and
one task taken all the way through `pickup → checkpoint → ship --dry-run → done`.

---

## Ground rules for you, the agent

Read these before running anything.

1. **Never replace an existing `jeff` binary on PATH without asking.** If JEFF is
   already installed, a crew of workers may be running against that exact binary;
   swapping it mid-session kills them. Upgrade only with explicit confirmation, and
   never `go build -o jeff` into a PATH directory.
2. **Never delete or overwrite an existing JEFF home.** If `jeff init` reports JEFF is
   already initialized, that is not an error to work around — switch to
   `jeff init --update` and tell the human what you found.
3. **One question at a time, batched where possible.** Every `ASK` block below is a
   real decision point. Present the default, let the human accept it with one word.
4. **Verify, don't assume.** Each step has a verification command. Run it. If output
   does not match, stop and show the human the actual output.
5. **Never invent task IDs.** IDs like `gig-ab12` in docs are placeholders. Always use
   an ID that a `gig create` you actually ran printed back.
6. **Don't paste secrets into config.** JEFF's `jeff.json` is plain text. Tokens belong
   in the environment or the credential helpers `git`/`gh` already use.
7. **Flag the permissions default explicitly** (Step 8). JEFF launches agents with
   their native permission prompts *disabled* by default. The human must know.

---

## Step 0 — Survey the machine (read-only)

Run this first and keep the output; later steps branch on it.

```bash
uname -s                                    # Darwin | Linux | MINGW*/MSYS*
command -v jeff gig git tmux gh jq          # what's already here
command -v claude opencode gemini codex           # which agent CLIs exist
jeff --version  2>/dev/null || echo "jeff: not installed"
gig  --version  2>/dev/null || echo "gig: not installed"
cat ~/.config/jeff/home 2>/dev/null || echo "no JEFF home pointer"
echo "JEFF_HOME=${JEFF_HOME:-<unset>}  GIG_HOME=${GIG_HOME:-<unset>}"
```

Interpret:

| Observation | What it means |
|---|---|
| `~/.config/jeff/home` exists and points at a dir with `jeff.json` | JEFF is **already initialized**. Skip to Step 3 in `--update` mode. |
| `~/.config/jeff/home` exists but target dir is missing | **Dangling pointer.** The pointer references a deleted dir. Confirm with the human, then `jeff init` to create a fresh home. |
| `JEFF_HOME` is set | It wins over the pointer file. Use it as the home for every later step. (`jeff init` also respects `JEFF_HOME`.) |
| No agent CLI found | Step 4 must install one, or JEFF has nothing to launch. |
| No `tmux` | Solo mode works fine. Crew mode (Step 10) does not. |
| No `gh` | `jeff ship` will fail (it fast-fails unless `--dry-run`). Note it; don't block. |

---

## Step 1 — Install gig (the task database)

JEFF cannot function without a gig store. `jeff doctor` checks for gig in its
dependency and environment sections, but run it explicitly here so you see the
gig configuration before the rest of the setup.

**macOS / Linux with Homebrew:**
```bash
brew install neerajg03/tap/gig
```

**Any platform with Go ≥ 1.24:**
```bash
go install github.com/NeerajG03/gig/cmd/gig@latest
```

Verify:
```bash
gig --version
```

If `go install` succeeded but `gig` is not found, `$(go env GOPATH)/bin` is not on
PATH. Tell the human the exact line to add to their shell rc:
```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

---

## Step 2 — Initialize the gig store

> **ASK:** "What prefix should your task IDs use? gig will generate IDs like
> `<prefix>-a3f8`. Default: `gig`. Common choices are a short company or project
> name." — Accept a short lowercase alphanumeric answer; default to `gig`.

```bash
gig init --prefix <answer>
```

Verify — this must print a table (empty is fine), not an error:
```bash
gig list
```

Notes for you:
- gig's SDK falls back to defaults when no config file exists, so JEFF *appears* to
  work without `gig init`. Run it anyway: the prefix is baked into every task ID and
  changing it later leaves you with mixed IDs.
- If the human already had a gig store, **do not re-init**. `gig init` on an existing
  store is not something to try casually — confirm first with `gig list`.

---

## Step 3 — Install and initialize JEFF

### 3a. Install (skip if `jeff --version` already worked)

```bash
brew install NeerajG03/tap/jeff
# or
go install github.com/NeerajG03/JEFF/cmd/jeff@latest
```

If `jeff` already exists and the human wants an upgrade, confirm first (Ground rule 1),
then check for running workers before touching it:
```bash
tmux ls 2>/dev/null | grep '^jeff-' || echo "no orchestrator sessions running"
```

### 3b. Initialize the home

Fresh machine:
```bash
jeff init
```

Already initialized (pointer file existed in Step 0):
```bash
jeff init --update
```

> **ASK, only if the human raised it or the machine is shared:** "Put the JEFF home at
> the default `~/.jeff/`, or somewhere else?" — For a non-default location, export
> `JEFF_HOME=<path>` *before* running `jeff init`, and tell the human to add that
> export to their shell rc. `jeff init --here` creates `./jeff/` in the current
> directory instead — only use it if they explicitly want a project-local home.

Verify:
```bash
jeff config          # prints agent / ide / home
ls "$(cat ~/.config/jeff/home)"
```

Expected directories: `repos/ tasks/ worktrees/ exports/ scripts/ projects/ .skills/
.personas/` plus `hooks/`, `memory/`, `personas/`, `proposals/`, `queue/`,
`transcripts/`, `archive/`, `CLAUDE.md`, `jeff.json`, and per-agent dirs
(`.claude/`, `.opencode/`, `.gemini/`).

---

## Step 4 — Check dependencies

```bash
jeff doctor
```

`jeff doctor` reports `gig`, `tmux`, `git`, `jq`, `gh`, `terminal-notifier`, and the
agent CLIs in its dependency section, plus environment checks including gig
initialization. One thing you must correct for — its install hints are Homebrew-only.
On Linux, translate:

| Dep | Debian/Ubuntu | Fedora | Arch |
|---|---|---|---|
| tmux | `sudo apt install tmux` | `sudo dnf install tmux` | `sudo pacman -S tmux` |
| jq | `sudo apt install jq` | `sudo dnf install jq` | `sudo pacman -S jq` |
| git | `sudo apt install git` | `sudo dnf install git` | `sudo pacman -S git` |
| gh | [cli.github.com](https://cli.github.com/manual/installation) | `sudo dnf install gh` | `sudo pacman -S github-cli` |

`terminal-notifier` is macOS-only — ignore it elsewhere.

Install every **required** dep that is missing. `gh` is optional for setup but
required later by `jeff ship`; if it is missing, say so now rather than at ship time.
If `gh` is present, confirm it is authenticated:
```bash
gh auth status
```

---

## Step 5 — Choose the agent CLI

> **ASK:** "Which agent CLI should JEFF launch? Options: `claude` (Claude Code),
> `opencode`, `gemini`, `codex` (OpenAI Codex). I found these installed: `<list from Step 0>`."

Default to `claude`. **The default in a fresh `jeff.json` is `claude` whether or not
Claude Code is installed** — so if the human picked something else, you must set it
explicitly:

```bash
jeff config agent opencode      # or claude | gemini | codex
```

If the chosen CLI is not installed:
```bash
npm install -g @anthropic-ai/claude-code    # claude
npm install -g @google/gemini-cli           # gemini
npm install -g @openai/codex                # codex
# opencode: follow https://github.com/anomalyco/opencode
```

For `opencode` only — if the human wants short `--model` aliases:
```bash
jeff config opencode add k2 opencode-go/kimi-k2.7-code
```
Warn them: registering the first alias means *only* registered aliases and real
`provider/model` ids are accepted for opencode from then on.

Verify:
```bash
jeff config          # agent line matches the answer
```

---

## Step 6 — Choose the IDE

> **ASK:** "Which editor should `jeff open` launch? `vscode`, `cursor`, `windsurf`,
> `nvim`, or `zed`?"

```bash
jeff config ide cursor
```

Unset means `vscode`. Verify with `jeff config ide`.

---

## Step 7 — Register repos

This is the step that makes JEFF useful. Every task workspace is built from registered
repos.

> **ASK:** "Which repositories will you work on? Give me clone URLs (SSH or HTTPS).
> You can add more later with `jeff repo add`."

For each URL:
```bash
jeff repo add <url>                  # name derived from the URL
jeff repo add <url> --name backend   # or set a short name explicitly
```

JEFF clones into `JEFF_HOME/repos/<name>`. If a clone fails, it is almost always
credentials — check that the human can `git clone` that URL themselves first; do not
try to work around auth.

Then, per repo, offer these (all optional):

```bash
jeff repo describe backend "Go API service"        # helps agents pick the right repo
jeff repo post-setup backend scripts/setup.sh      # runs on every new worktree
```

A post-setup script receives two arguments — `src_dir` (the repo clone) and `dest_dir`
(the new worktree) — and is the right place to copy `.env` files or symlink
`node_modules`. If the human wants one, create it under `JEFF_HOME/scripts/`,
`chmod +x` it, and register it.

> **ASK, per repo, only if they branch off something other than `main`:** "What base
> branch should new task branches fork from?" Set it by editing `jeff.json`'s
> `repos.<name>.base_branch` (e.g. `origin/develop`) — there is no CLI flag for this
> yet.

Verify:
```bash
jeff repo list
jeff repo sync           # confirms every clone is reachable
```

---

## Step 8 — Hooks and the permissions default

### 8a. Hooks

```bash
jeff config hooks list
```

**All hooks are enabled by default** — a hook is on unless `jeff.json` explicitly sets
it to `false`. They inject task context, repo lists, gig instructions, checkpoint
nudges, and crew inbox delivery into agent sessions. The sane default is: leave them
alone.

Only if the human reports noise:
```bash
jeff config hooks disable gig-ready-tasks
jeff config hooks sync
```

### 8b. The permissions default — say this out loud

Tell the human, in plain words:

> JEFF launches agents with their native permission prompts **disabled** by default
> (`skip_permissions: true`). That means an agent in a task workspace can run commands
> and edit files without asking each time. It is fast, and it is a real trust
> boundary.

> **ASK:** "Keep permission prompts disabled (default, faster), or enable them
> (safer)?"

To enable prompts globally, set `"skip_permissions": false` in `jeff.json`. Either way,
tell them about the per-invocation override:
```bash
jeff pickup <id> --safe        # also on: jeff work, jeff crew start
```

---

## Step 9 — Tour personas, skills, and memory

Do not configure these deeply on day one. Show the human what exists so they know the
levers, then move on to a real task.

### Personas — who the agent is

```bash
jeff persona list
jeff persona show jenko
```

Six ship in the binary: **jenko** (implementer, default), **schmidt** (debugger),
**dickson** (orchestrator — plans and delegates, writes no code), **eric** (researcher
— changes no code), **hardy** (reviewer), **marlowe** (memory curator, used by
`jeff memory curate`). Selected per task with `--persona`. Custom ones:
`jeff persona add <path>`.

### Skills — reusable instructions, auto-injected

```bash
jeff skill list
jeff skill doc            # how to author a skill
```

Skills are `SKILL.md` directories symlinked into task workspaces when they match the
task's persona or tags. Five skills are embedded in the binary and installed by
`jeff init`: `crew-orchestrator`, `curation`, `go-testing`, `pr-review`, `root-cause`.
The last three are persona-tagged (`jenko`, `hardy`, `schmidt`) so they auto-inject
when the matching persona picks up a task.

```bash
jeff skill add ./my-skill                 # register
jeff skill tag my-skill --persona jenko   # auto-inject for that persona
jeff skill inject slack notion            # inject into the JEFF home itself
```

### Memory — what carries across sessions

```bash
jeff memory list
```

Three layers: **persona memory** (per-persona, crosses tasks), **repo learnings**
(per-repo quirks), and the per-task **scratchpad**. During a session an agent runs
`jeff memory propose` to record an observation; later `jeff memory curate` runs the
marlowe persona to consolidate proposals into canonical memory. Tell the human that
curation is a deliberate, human-triggered step — proposals do not auto-promote.

---

## Step 10 — Take one real task end to end

Do not declare setup finished until this loop completes. It is the only proof the
whole chain works.

```bash
# 1. Create a real task — capture the ID it prints.
gig create "Verify JEFF setup end to end" --type chore --priority 3
# → e.g. "Created myapp-7c1e"

# 2. Claim it and open a workspace (use the printed ID and a registered repo name).
jeff pickup myapp-7c1e --persona jenko --repos backend
```

`jeff pickup` claims the task in gig, creates `JEFF_HOME/tasks/<id>/`, creates a git
worktree branched from the repo's base branch, symlinks matching skills, writes a
task `CLAUDE.md` with persona + task context, and launches the agent.

> **Note:** In a non-interactive context (CI, agent-driven setup), add `--test` to
> prepare the workspace and verify its structure without launching the agent:
>
> ```bash
> jeff pickup myapp-7c1e --persona jenko --repos backend --test
> ```
>
> `--test` claims the task, creates the workspace and worktrees, wires skills, writes
> the task CLAUDE.md, then prints the paths and exits so you can inspect them:
>
> ```
> Test mode — workspace ready at /path/to/tasks/gig-fd55-...
> Verify:
>   • Task dir:   /path/to/tasks/gig-fd55-...
>   • CLAUDE.md:  /path/to/tasks/gig-fd55-.../CLAUDE.md
>   • Worktrees:  ls /path/to/tasks/gig-fd55-.../
>   • Skills:     ls /path/to/tasks/gig-fd55-.../.claude/skills/
> ```
>
> After verifying, continue with the commands below as normal.
>
> If you do want the interactive session, omit `--test` — and if you are running in a
> terminal, `jeff pickup` opens the agent directly.

```bash
# 3. From anywhere, inspect state.
jeff status
jeff open myapp-7c1e            # opens the workspace in the configured IDE

# 4. Record progress (run inside the task dir, or pass --task).
jeff checkpoint --done "Confirmed setup works" --next "Start real work"

# 5. Rehearse shipping — dry run makes no PR and needs no gh auth.
jeff ship --dry-run

# 6. Close it out.
jeff done myapp-7c1e --reason "setup verified"
```

Verify the loop closed:
```bash
gig show myapp-7c1e      # status closed, checkpoint recorded
jeff status --all
```

`jeff done` refuses to discard a worktree with uncommitted changes unless you pass
`--force`. If it refuses, that is correct behavior — show the human, let them decide.

---

## Step 11 — Crew mode (optional, only if they want parallel agents)

> **ASK:** "Do you want multi-agent crew mode — several agents working different tasks
> in parallel, each in its own tmux window, coordinated by an orchestrator? It needs
> tmux ≥ 3.0."

If no, skip to Step 12.

**The step almost everyone misses:** `jeff crew start` hard-fails unless an
orchestrator identity is registered *in the current directory*. Run `init` first.

```bash
cd <the project directory you want to orchestrate from>
jeff orchestrator init                  # registers a durable orchestrator identity here
jeff orchestrator start --name work     # creates tmux session "jeff-work"
```

Then, from inside that session:
```bash
jeff crew start <task-id> "Fix the auth bug" --persona jenko --repos backend
jeff crew list
jeff crew status <task-id>
jeff crew send <task-id> "also add error handling"
jeff crew send <task-id> "stop, switch to payments" --interrupt
jeff dashboard                          # live TUI, refreshes every 2s
```

Workers can ask questions back (`jeff crew ask`), and the orchestrator answers with
`jeff crew ack <msg-id> "..."`. Wind down with `jeff crew stop --all` and
`jeff orchestrator stop jeff-work`.

If `jeff crew start` errors with *"no orchestrator identity found"*, you skipped
`jeff orchestrator init` in this directory. Run it there — do not set
`JEFF_ORCHESTRATOR_ID` to work around it unless the human has a registered ID already.

---

## Step 12 — Finish

Shell completions:
```bash
jeff completion zsh  > "${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/completions/_jeff"  # zsh (oh-my-zsh)
jeff completion zsh  > /opt/homebrew/share/zsh/site-functions/_jeff            # zsh (Homebrew)
jeff completion bash > /etc/bash_completion.d/jeff
jeff completion fish > ~/.config/fish/completions/jeff.fish
```

Then report back to the human with a summary in this exact shape:

```
JEFF is ready.

  home     <path>
  agent    <claude|opencode|gemini>
  ide      <vscode|cursor|windsurf|nvim>
  gig      prefix <prefix>, store at <path>
  repos    <name> (<url>), ...
  crew     configured | not configured
  perms    prompts disabled (default) | prompts enabled

Verified: created <task-id>, picked it up, checkpointed, ship --dry-run, closed it.

Next:
  gig create "your first real task"
  jeff pickup <id> --persona jenko --repos <name>

Not done / needs your attention:
  - <anything you skipped, and why>
```

Be explicit about what you skipped. A setup that silently omitted `gh` auth or crew
mode is worse than one that names the gap.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `JEFF is not initialized. Run: jeff init` | No home pointer and no `JEFF_HOME` | `jeff init` |
| `JEFF is already initialized at <path>` | Pointer points elsewhere | `JEFF_HOME=<path> jeff init --update` to update the right home, or `rm ~/.config/jeff/home` only if the human confirms that home is dead |
| `resolve JEFF_HOME` / `load config` errors | `JEFF_HOME` points at a dir with no `jeff.json` | Fix the env var, or `jeff init` at that path |
| `jeff pickup` can't find the repo | Name mismatch | `jeff repo list` — use the short name, not the URL |
| `jeff ship` fails immediately | `gh` missing or unauthenticated | Install `gh`, `gh auth login`; `--dry-run` needs neither |
| `no orchestrator identity found` | `jeff orchestrator init` not run in this dir | Run it there (Step 11) |
| Agent launches without task context | Hooks out of sync | `jeff config hooks sync --tasks` |
| Tasks appear under the wrong ID prefix | gig store initialized after tasks existed, or two stores | `gig config` / check `GIG_HOME`; JEFF's own store resolution ignores `jeff.json`'s `gig_home` today, so prefer `GIG_HOME` |
| Skills not appearing in a workspace | Not tagged for that persona | `jeff skill tag <name> --persona <persona>`, then re-run pickup |
| `jeff pickup` exits with a usage block after claiming the task | Agent tool failed to launch (e.g. no stdin in non-interactive shell) | Workspace was created successfully. Use `jeff pickup <id> --test` to verify structure, or `jeff work <id>` to resume without re-launch |
| Stale worker/tmux state | Crashed workers | `jeff crew cleanup` |

---

## Command reference for this runbook

Everything used above, in order of first use:

```
gig init --prefix <p>            gig create "<title>"        gig list / gig show <id>
jeff init [--update|--here]      jeff doctor                 jeff config
jeff config agent <a>            jeff config ide <i>         jeff config hooks list|sync
jeff config opencode add <n> <p/m>
jeff repo add <url> [--name]     jeff repo list|sync         jeff repo describe|post-setup
jeff persona list|show|add|tag   jeff skill list|doc|add|tag|inject
jeff memory list|propose|curate
jeff pickup <id> [--persona] [--repos] [--safe] [--test]     jeff work [id]
jeff status [--all]              jeff open [id]              jeff checkpoint --done "..."
jeff ship [--dry-run|--draft]    jeff done <id> [--reason] [--force]
jeff orchestrator init|start|list|info|attach|stop
jeff crew start|list|status|send|ask|ack|events|capture|resume|stop|cleanup|attach
jeff dashboard                   jeff stats                  jeff completion <shell>
```

Deeper references: [`docs/usage.md`](https://github.com/NeerajG03/JEFF/blob/main/docs/usage.md)
· [`docs/config.md`](https://github.com/NeerajG03/JEFF/blob/main/docs/config.md)
