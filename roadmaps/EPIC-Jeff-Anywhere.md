# EPIC-Jeff-Anywhere: JEFF as a Distributed Worker System with Chat Gateways

> **Type:** Epic (multi-phase) · **Leverage:** Strategic — turns JEFF from a laptop tool into a place work gets done · **Effort:** Large, but phased so every phase ships standalone value
>
> This document is the architecture + phased execution plan for: *"run JEFF as an agent wherever I want, clients connect via a Slack app or a JEFF app with a chat-like UI, all instances collectively share memory/personas/skills/hooks, and any active worker can pick up a task — JEFF as a worker."*

## The vision, restated as requirements

1. **JEFF-as-a-worker**: a headless `jeff worker` process runs on any machine (home server, VPS, office Mac, cloud box). It claims tasks it is capable of, does the full pickup → agent → checkpoint → ship lifecycle, and reports back. If a worker is offline, another picks up the task. ("As long as one active worker can pick it up.")
2. **Chat clients**: you talk to JEFF from Slack (first) or a JEFF chat UI (later). A conversation thread ≙ a task. You create work, get progress, answer the agent's questions, and approve shipping — all from chat.
3. **Collective context**: memory, personas, skills, and hooks are shared. A learning curated from a task run on machine A benefits the next task on machine B.
4. **Run anywhere**: no inbound ports on workers or your laptop. Everything dials out.

## Architecture (recommended): hub-and-spoke

```
                    ┌──────────────────────────────────┐
   Slack (Socket    │            jeff hub              │
   Mode, dials ──►  │  • authoritative gig store       │
   out to Slack)    │  • canonical JEFF_HOME           │
                    │    (memory / personas / skills)  │
   Web chat UI ──►  │  • worker registry + leases      │
   (later, served   │  • message bus (WS)              │
   by hub)          │  • marlowe curation runs here    │
                    └───────────────┬──────────────────┘
                                    │  workers dial OUT (WSS + token)
              ┌─────────────────────┼─────────────────────┐
              ▼                     ▼                     ▼
       jeff worker            jeff worker            jeff worker
       (office Mac)           (home server)          (cloud VM)
       repos: backend         repos: backend,        repos: infra
       agents: claude         frontend               agents: claude,
                              agents: claude,        gemini
                              gemini
       • local workspace/worktrees (existing code)
       • headless agent runs (claude -p --output-format stream-json)
       • git+gh credentials live HERE, never on the hub
```

### Why hub-and-spoke and not peer-to-peer sync (decision record)

Two facts about gig v0.6.2 force this (both verified in the module source):

1. **`Claim` is get-then-update, not compare-and-swap** (`task.go:458-495`: `Get` then unconditional `UPDATE tasks SET assignee=..., status=...`). Two concurrent claimers both "succeed"; last write wins. Claiming is only safe if **one process serializes it**. A hub that owns the store and arbitrates claims behind a mutex gives exactly that. (Peer-synced SQLite — litestream/turso/git-synced DBs — cannot fix an application-level race.)
2. **`store.On` events fire in-process only** (`store.go:149-165`: `emit` runs registered callbacks in the writing process). The roadmap's Phase-5 daemon design ("event loop over gig's store, no polling") works *only* if all writes flow through the process hosting the loop. Hub = that process.

Consequences embraced: the hub is a single point of coordination (fine — it's tiny, stateless workers reconnect on restart), and remote CLI/gig access goes through the hub's API rather than opening the SQLite file over a network share (which SQLite forbids anyway).

### Other locked decisions

| Decision | Choice | Why |
|---|---|---|
| Worker transport | WebSocket, workers dial out, token auth | "run anywhere" = no inbound ports on workers; reconnect semantics are well-understood |
| Slack integration | **Socket Mode** | no public URL needed — the hub also only dials out; a JEFF hub on a home server can drive a Slack workspace |
| Agent execution on workers | headless (`-p` + streaming output), per-task workspace exactly as today | chat-driven work has no tty; tmux stays for *local* interactive use, unchanged |
| Where memory curation runs | hub only (marlowe, `JEFF_MEMORY_CAN_ADD=1`) | preserves the single-writer invariant the memory package is built on (`memory/CLAUDE.md`) |
| Context distribution | "context pack" (tar) built by hub at claim time; proposals POSTed back | maps 1:1 onto the existing file-based inject/propose design; no live sync needed |
| Git/gh credentials | workers only | hub never holds secrets that can push code; capability advertising ("I have repo X") replaces central creds |
| Chat UI | Slack first, then a small web UI served by the hub reusing the same protocol | Slack is a finished, mobile-ready chat client; build zero UI to get to useful |

## What already exists to build on (the reuse map)

This epic is mostly *transport*, because the domain layer is already shaped right:

| Existing piece | Role in JEFF-Anywhere |
|---|---|
| `gig.Claim/Ready/Events/Checkpoints/Attrs` | the task queue, audit log, and progress stream — hub-side, unchanged |
| `crew` message types `nudge/status/normal/divert` (`crew/crew.go:60-91`) | the chat→agent message vocabulary, reused verbatim over WS |
| crew's local store + `inbox-check` hook (`hooks/builtin.go:380-430`) | **remote delivery for free**: the worker daemon writes incoming hub messages into its *local* crew store; the existing PostToolUse hook injects them into the running agent. Zero new hook mechanics |
| memory v1 `proposals/ → queue/ → curate` (`memory/`) | the distributed learning loop — workers produce proposals locally, POST them to the hub queue; marlowe curates at the hub exactly as it curates today |
| `AgentProvider` (`agent.go`) | grows one method pair for headless streaming runs; all agents inherit |
| task workspace/worktree code (`workspace/`) | runs unchanged on each worker — workspaces were always local and ephemeral ("task dirs are ephemeral — gig is the source of truth") |
| checkpoint injection on resume (PLAN-Phase1-Attrs-Resume) | **the cross-worker resume story**: agent CLI sessions are machine-local, so when task moves between workers, checkpoints are what reconstruct context |

### Prerequisites from the top-10 (do these first — they are load-bearing here)

- **PLAN-Gig-Upgrades** (separate repo, `roadmaps/PLAN-Gig-Upgrades.md`) — gig itself needs hardening for multi-client use: the same pooled-PRAGMA bug crew has, a compare-and-swap `Claim` with `ErrAlreadyClaimed` (defense in depth under the hub's mutex, and it fixes local crew double-claim races today), transactional event recording (events are the worker progress stream — currently droppable), ID-collision retry + longer default IDs, and an `EventsAfterID` cursor the hub's sweep loop uses. Ship as gig v0.7.0 first.

- **#5 PLAN-Pickup-Rollback** — the worker's core loop body IS `task.Pickup`. It must exist as a library function, and (amendment, see below) take a narrow store interface. Rollback/idempotent-resume also becomes lease-recovery behavior.
- **#1 PLAN-Phase1-Attrs-Resume** — checkpoints + attrs are the only portable task state; cross-worker resume is impossible without checkpoint injection.
- **#2 PLAN-Crew-Reliability** — the worker daemon writes to the local crew store concurrently with hooks; the pragma/transaction fixes must land first.
- **#8 PLAN-Agent-Providers** — headless args become one more provider method instead of another string switch.
- **#7 PLAN-Permission-Safety** — headless runs require skip-permissions by definition; the config knob documents that trust boundary instead of hiding it.

**Amendment to PLAN-Pickup-Rollback (apply when executing it):** define `task.Pickup` against a minimal interface rather than `*gig.Store`:

```go
// task.Store is the slice of gig the pickup/teardown lifecycle needs.
// *gig.Store satisfies it; remote workers pass an HTTP-backed implementation.
type Store interface {
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
```

One caveat to design around: on a **remote** worker, `Claim` must NOT be called by the worker at all — the hub arbitrates claims (see protocol). The worker-side `task.Pickup` runs in "already claimed" mode (the self-heal path PLAN-Pickup-Rollback already specifies: in_progress + no workspace → proceed without re-claiming). That path stops being an edge case and becomes the remote worker's main line.

---

## Phase A — `jeff hub` + `jeff worker` (the foundation)

**Goal:** one hub, one worker, no Slack yet. `jeff hub serve` on machine 1; `jeff worker run --hub wss://host --token T` on machine 2. Create a task at the hub (`gig create` on the hub machine or `jeff hub task new`), watch the worker claim it, build its workspace, run the agent headless, stream checkpoints/output back, ship, and close. Everything after this phase is a client of what A builds.

### New packages and files

| File | Contents |
|---|---|
| `remote/protocol.go` | frame structs + (de)serialization for the WS protocol below; version constant |
| `remote/hub/hub.go` | WS server, worker registry, claim arbitration (single mutex), offer loop |
| `remote/hub/leases.go` | lease table + expiry sweep (SQLite, reuse the crew store's DB file with new `leases` table via the versioned-migration mechanism from PLAN-Crew-Reliability) |
| `remote/hub/packs.go` | context-pack builder (tar.gz of persona template + memory index + repo learnings + matched skills for a task) |
| `remote/hub/api.go` | minimal HTTP: `POST /v1/proposals`, `POST /v1/queue`, `GET /v1/packs/<task>.tgz`, `GET /healthz` |
| `remote/worker/worker.go` | dial/hello/reconnect loop, capability report, claim handling, heartbeats |
| `remote/worker/runner.go` | per-task: remote-store shim + `task.Pickup` + headless agent run + event streaming |
| `remote/worker/store.go` | `task.Store` implementation backed by WS/HTTP calls to the hub |
| `cmd/jeff/hub_cmd.go` | `jeff hub serve [--addr :8443] [--home <JEFF_HOME>]`, `jeff hub token new <name>`, `jeff hub workers`, `jeff hub send <task> <msg> [--type]` |
| `cmd/jeff/worker_cmd.go` | `jeff worker run --hub <url> --token <t> [--max-tasks 2] [--repos a,b]` |

Dependency: one WebSocket library — `github.com/coder/websocket` (pure Go, no CGO, maintained). No other new deps in Phase A.

### The protocol (JSON frames over one WS per worker)

```jsonc
// worker → hub, first frame
{"t":"hello","token":"...","worker":{"name":"office-mac","version":"0.9.13",
  "repos":["backend","frontend"],"agents":["claude"],"max_tasks":2}}

// hub → worker: a claimable task matching the worker's capabilities
{"t":"offer","task_id":"gig-ab12","title":"Fix auth bug","persona":"jenko","repos":["backend"]}

// worker → hub: I'll take it
{"t":"claim","task_id":"gig-ab12"}

// hub → worker: hub ran gig.Claim (serialized behind its mutex); includes lease + pack
{"t":"claim_result","task_id":"gig-ab12","ok":true,"lease_s":900,
  "pack_url":"/v1/packs/gig-ab12.tgz","task":{ /* gig show --json shape */ }}

// worker → hub: progress + gig writes (the remote task.Store rides these)
{"t":"event","task_id":"gig-ab12","seq":17,"kind":"checkpoint","data":{"done":"...","next":"..."}}
{"t":"event","task_id":"gig-ab12","seq":18,"kind":"output","data":{"text":"...chunk..."}}
{"t":"event","task_id":"gig-ab12","seq":19,"kind":"question","data":{"text":"JWT or session tokens?"}}
{"t":"event","task_id":"gig-ab12","seq":20,"kind":"shipped","data":{"prs":{"backend":"https://..."}}}
{"t":"event","task_id":"gig-ab12","seq":21,"kind":"done","data":{"reason":"done"}}

// hub → worker: a chat/user message for the running agent (crew vocabulary!)
{"t":"send","task_id":"gig-ab12","msg_type":"normal","content":"API spec changed, see thread"}

// worker → hub, every 30s; renews leases for listed tasks
{"t":"heartbeat","tasks":[{"id":"gig-ab12","status":"running"}]}

// either direction
{"t":"release","task_id":"gig-ab12","reason":"worker shutting down"}
{"t":"error","ref_seq":17,"msg":"..."}  // hub rejecting an event write
```

Rules a weaker model must not improvise around:
- `seq` is per-task, monotonically increasing, assigned by the worker; the hub persists the last applied seq per task and **drops duplicates** — this makes reconnect-and-resend idempotent.
- The **hub is the only caller of `gig.Claim`**, inside one mutex — this is the fix for the get-then-update race documented above. Offers go to all capable workers; first `claim` frame wins; others get `{"ok":false,"reason":"taken"}`.
- Workers buffer unacked `event` frames on disk (`JEFF_HOME/worker-outbox/<task>/<seq>.json`) and replay on reconnect.

### Hub internals

1. **Store ownership:** hub opens the gig store (existing `openGigStore` path against the hub's `GIG_HOME`) and keeps it open. It registers `store.On(gig.EventStatusChanged, ...)` to catch tasks becoming `open`, plus a 30s `Ready("")` sweep fallback (needed because CLI writes on the hub machine bypass the hub process and never fire its in-process callbacks — verified behavior of `emit`).
2. **Offer matching:** capability check = task's `repos` attr ⊆ worker.repos AND (persona's registered agent ∈ worker.agents, default claude). Persist nothing about offers; recompute on worker connect.
3. **Leases:** on claim, insert `lease(task_id, worker, expires_at)`. Heartbeats extend. A sweeper (1m tick) expires leases: task's gig status → back to `open`, assignee cleared (`UpdateStatus` + `Update{Assignee:""}` — the same un-claim pair PLAN-Pickup-Rollback uses), a comment recorded: `"lease expired on worker X — released"`. Any checkpoint the worker streamed before dying is already in gig — the next worker resumes from it (this is why PLAN-Phase1-Attrs-Resume is a prerequisite).
4. **Event application:** `checkpoint` → `store.AddCheckpoint`; `shipped` → `SetAttr(pr_urls)` + comment (same as PLAN-Ship-Hardening); `done` → `CloseTask`; `output` → ring buffer per task in hub memory (last 64KB) + optional log file `JEFF_HOME/task-logs/<id>.log` — NOT written to gig (events table is an audit log, not a firehose).
5. **Context packs:** tar.gz containing `PERSONA.md`, persona memory index + detail files, repo learnings for the task's repos, matched skills (reuse `skill.MatchAll`), and a `pack.json` manifest. Built lazily per claim, cached until memory changes (mtime check is fine at this scale).
6. **Auth:** `jeff hub token new <name>` generates a random 32-byte token, stores `sha256(token)` in the hub DB with the name; `hello` is checked against it. Transport security is deployment's job: document "bind to localhost and put a TLS reverse proxy (caddy/nginx) or tailscale in front; never expose the plain listener" in the command help.

### Worker internals

1. **Connect loop:** dial, `hello`, then serve frames; exponential backoff reconnect (1s→60s cap). On reconnect: re-`hello`, replay outbox, hub replies with which tasks it still considers leased to this worker — reconcile (continue or abandon local work).
2. **On `claim_result ok`:** download pack → materialize into a *worker-local* JEFF_HOME cache (`memory/`, `.skills/` etc. under `WORKER_HOME`), then run `task.Pickup` with (a) the remote `task.Store` shim, (b) "already claimed" mode. Workspace + worktrees are the existing local code paths; the repos must already be registered on the worker (`jeff repo add` at worker install time — creds stay local).
3. **Headless run:** extend `AgentProvider` with:

```go
// BuildHeadlessArgs returns args for a non-interactive streaming run.
// resumeID may be "" for a fresh session.
BuildHeadlessArgs(prompt, model, resumeID string) []string
```

For claude: `["-p", prompt, "--output-format", "stream-json", "--verbose", "--dangerously-skip-permissions"]` (+ `--model`, `--resume`). The runner parses stream-json lines: assistant text → `output` events (coalesced, ≤4KB chunks, ≥2s apart); result line → capture session id and exit. Gemini/opencode analogues can start as "run, capture combined output, emit at end" — streaming parity is not required for A.
4. **Incoming `send` frames** → written into the worker's **local crew store** as pending messages with the given `msg_type` (nudge semantics from PLAN-Crew-Reliability apply) → the existing `inbox-check` hook injects them on the agent's next tool use. `divert` in headless mode = kill the agent process, then start a new headless run with the divert content as the prompt and `--resume <session>`.
5. **Agent asks a question:** the crew `ask` flow (`jeff crew ask`) already writes a `to_orchestrator` message locally; the worker daemon tails its local crew store for those and forwards them as `question` events. (Hook mechanics unchanged — the daemon is the new "orchestrator pane".)
6. **Ship/done:** the agent (or the runner, on success policies later) runs `jeff ship`/`jeff done` locally — those need gig, which on a worker means the remote store shim: `cmd/jeff` grows an env-based switch `JEFF_HUB_URL`/`JEFF_HUB_TOKEN` + task scoping so `openGigStore()` returns the remote shim when set. This is the one genuinely invasive change in Phase A — do it as: `gig.go`'s `openGigStore` returns the `task.Store` interface, remote when env is set. Commands that need APIs outside the interface (e.g. `crew events`) simply don't work in worker mode and must error clearly ("not available against a hub — run on the hub machine").

### Phase A edge cases (a weaker model will miss these)

- **Do not let the worker call `Claim`** — even via the shim. The shim's `Claim` returns an error ("claims are hub-arbitrated"); `task.Pickup`'s already-claimed path is the entry.
- **Duplicate offers:** a task can be offered, declined-by-silence, and re-offered on the next sweep; workers must treat repeat offers idempotently.
- **Lease expiry vs. slow-but-alive worker:** heartbeat carries per-task status; only expire when heartbeats stop, not when the task is merely long-running (the lease renews on every heartbeat — expiry means the *connection* died, and reconnect+outbox handles the grace window; set lease = 3× heartbeat + jitter, i.e. ~120s, not 900s as sketched — tune once, put it in one const).
- **The `release` after partial work:** on lease expiry the hub must NOT delete anything the worker streamed; checkpoints are additive. Re-offer includes a note in the offer frame (`"resumed":true`) so the next worker's pickup regenerates CLAUDE.md with the checkpoint (which PLAN-Phase1 already does).
- **Clock skew:** leases are hub-clock relative only (`expires_at` computed hub-side). Workers never compare timestamps.
- **Output flooding:** cap `output` events (chunk, rate-limit, truncate middle beyond 64KB per task with an explicit `[...truncated...]` marker) — Slack and the hub ring buffer both need this later.
- **Worker-local JEFF_HOME collision:** a machine can run interactive JEFF and a worker; give the worker its own home (`--home`, default `~/.jeff-worker`) so pack materialization never clobbers the human's memory files.
- **SQLite at the hub:** the hub process owns the gig store, but `gig`/`jeff` CLIs on the hub machine share the file — WAL makes that safe for reads/writes, but hub in-process event callbacks won't fire for CLI writes; the 30s sweep covers it (already specified — do not "optimize" the sweep away).
- **Secrets in packs:** memory files may contain sensitive learnings; packs are served only to token-authenticated workers over the (TLS-fronted) listener, and the pack endpoint must check the task is actually leased to the requesting worker.

### Phase A acceptance criteria

1. `go build ./... && go vet ./... && go test ./...` green; new packages have unit tests: protocol round-trip, lease expiry (fake clock), seq dedupe, pack builder contents, claim arbitration under 10 concurrent fake workers (exactly one wins).
2. Two-process smoke test (single machine, two JEFF_HOMEs, scripted in `remote/e2e_test.go` behind `-short` skip): hub with a fixture task + fake provider agent (a stub `AgentProvider` registered in the test that echoes and exits) — worker claims, runs, checkpoint lands in the hub's gig store, task closes.
3. Kill the worker mid-task → within 2× lease the task is back to `open` with a release comment; restart worker → it re-claims and completes.
4. `jeff hub send gig-X "hello" --type normal` reaches the worker's local crew store (assert via `jeff crew inbox` on the worker).
5. No secrets on the hub: grep the hub codepaths for git credential use → none; ship happens worker-side.

---

## Phase B — Slack gateway (`jeff slack`)

**Goal:** a Slack app (Socket Mode) run by the hub process (`jeff hub serve --slack` reading `SLACK_APP_TOKEN`/`SLACK_BOT_TOKEN`) that maps threads ↔ tasks.

- **Dependency:** `github.com/slack-go/slack` (socketmode). Both tokens from env only — never config files.
- **Mapping:** a `slack_thread` gig attr (`channel:thread_ts`). Slash command `/jeff new <title>` (or @jeff mention) → `store.Create` + reply-in-thread "gig-ab12 queued". All later thread replies → `send` frames (default `msg_type normal`) to whichever worker holds the lease; no lease → queued in the hub and flushed on claim.
- **Outbound:** `checkpoint` events → thread message (`Done/Next/Blockers` formatted); `question` events → thread message with `@author` mention; `shipped` → PR links; `done` → ✅ + reaction on the root message. `output` events are NOT posted by default (noise) — a thread user can say `/jeff tail` to get the last output chunk.
- **Approvals (the roadmap's trust layer, finally real):** `shipped` posts *Approve & close* / *Send back* buttons (Slack interactivity). Approve → `CloseTask`; Send back → a `send` frame with `msg_type divert` containing the reviewer's comment, and increment the `rejection_count` attr (defined in PLAN-Phase1-Attrs-Resume; this is its first writer — and it makes `jeff stats`' rejection metrics live).
- **Edge cases:** Slack retries events (dedupe by `event_id` — keep a small LRU); 1 msg/sec/channel rate limit (queue + coalesce checkpoint spam); 40k char message cap (chunk); thread_ts of the *root* message must be stored, not of replies; bot must ignore its own messages (`bot_id` check); DM channels work the same (channel id starts with `D`).
- **Acceptance:** manifest checked into `docs/slack-manifest.yml` (socket mode on; scopes: `app_mentions:read`, `chat:write`, `commands`, `im:history`, `reactions:write`); an e2e doc walkthrough; unit tests for the thread↔task mapper and the event→message formatter (pure functions, no Slack calls).

## Phase C — fleet semantics

- **Routing rules** from the roadmap Phase 5 land here, hub-side, in the hub's config: match on type/labels/priority → require persona/repos (the `jeff.json` `routing` block sketched in `docs/roadmap.md:544-566`).
- **Worker affinity:** prefer re-offering a resumed task to the worker that last held it (agent CLI session ids are machine-local; same worker can `--resume` the actual conversation, others rebuild from checkpoints). Store `last_worker` on the lease row.
- **Concurrency and drain:** `--max-tasks` enforcement, `jeff worker drain` (finish current, take no more), hub `jeff hub workers` shows fleet state.
- **Priorities:** offer highest gig priority first; starvation guard (age bumps).

## Phase D — the distributed learning loop

- Workers POST `proposals/` files and session queue entries (`RunSessionEnd` output) to `POST /v1/proposals` / `/v1/queue` instead of (in addition to) local drops — one new transport branch in the worker daemon, zero changes to the memory package.
- Curation stays hub-only: a scheduled `jeff memory curate` (cron or hub timer) with marlowe; PLAN-Memory-Correctness's fixes (no-archive-on-error, `.last-curated`, dedupe) are prerequisites or this loop silently eats observations.
- Packs pick up new canonical memory automatically (mtime-based cache invalidation from Phase A).
- Result: the user's requirement "collectively share memory/persona/skills" is closed with ~200 lines of transport.

## Phase E — the JEFF chat UI (optional, after Slack proves the loop)

A single static page served by the hub (`/ui`) speaking the same WS protocol with a `client` role: task list (gig Ready/List via hub API), a thread view per task fed by the hub's ring buffer + gig events, a send box. No framework needed; keep it embedded (`//go:embed`). The protocol was designed in Phase A so this is a consumer, not a new system. (If more is wanted later, this is also the natural place for `jeff stats` dashboards.)

## Deployment: simple and config-driven (`jeff up`)

The rule: **one config block decides what a machine is; one command starts it; secrets never live in config files.** No flags needed in steady state — flags exist only to override for debugging.

### The `anywhere` block in `jeff.json`

```jsonc
// Hub machine (~/.jeff/jeff.json)
{
  "anywhere": {
    "role": "hub",
    "listen": "127.0.0.1:8443",          // bind; put TLS/tailscale in front
    "public_url": "wss://jeff.example.com", // what workers and gateways dial
    "lease_seconds": 120,
    "slack": { "enabled": true },          // tokens come from env, never here
    "routing": [                            // roadmap Phase-5 rules, hub-side
      {"match": {"type": "bug", "labels": ["backend"]},
       "action": {"persona": "jenko", "repos": ["backend"]}}
    ]
  }
}

// Worker machine (JEFF_HOME=~/.jeff-worker/jeff.json)
{
  "repos": { "backend": {"url": "git@github.com:org/backend.git"} },
  "anywhere": {
    "role": "worker",
    "hub": "wss://jeff.example.com",
    "token_file": "~/.jeff-worker/hub-token",  // 0600; or token_env: "JEFF_HUB_TOKEN"
    "max_tasks": 2,
    "advertise_repos": []                       // empty = all registered repos
  }
}
```

`jeff up` reads `anywhere.role` and becomes the right thing:
- `role: hub` → `hub serve` (+ Slack gateway if `slack.enabled` and both `SLACK_APP_TOKEN`/`SLACK_BOT_TOKEN` env vars are present; enabled-but-missing-tokens is a startup **error**, not a silent skip).
- `role: worker` → `worker run` against `anywhere.hub`.
- no `anywhere` block → friendly error pointing at the docs (never guess a role).

`jeff hub serve` / `jeff worker run` remain as explicit subcommands; `jeff up` is sugar so provisioning is copy-paste identical everywhere.

### Provisioning recipes (documented in `docs/anywhere.md`, shipped with Phase A)

Worker on any Linux box (systemd):

```ini
# /etc/systemd/system/jeff-worker.service
[Unit]
Description=JEFF worker
After=network-online.target
[Service]
Environment=JEFF_HOME=/home/jeff/.jeff-worker
ExecStart=/usr/local/bin/jeff up
Restart=always
RestartSec=5
User=jeff
[Install]
WantedBy=multi-user.target
```

Hub via docker compose:

```yaml
services:
  jeff-hub:
    build: .            # or a published image once releases exist
    command: jeff up
    volumes: ["jeff-home:/root/.jeff"]
    environment: ["SLACK_APP_TOKEN", "SLACK_BOT_TOKEN"]
    ports: ["127.0.0.1:8443:8443"]   # TLS terminates in front (caddy/tailscale)
    restart: unless-stopped
volumes:
  jeff-home:
```

Bootstrap flow, end to end: `jeff init` on the hub box → `jeff hub token new office-mac` (prints the token once) → on the worker: `jeff init`, `jeff repo add …`, write the 6-line `anywhere` block + token file → `systemctl enable --now jeff-worker`. Done — the worker appears in `jeff hub workers`.

Config notes for the executor: the `anywhere` block is one new optional struct on `Config` (`config.go`), with the same pointer-for-unset conventions as `skip_permissions`; add it to `schemas/jeff-config.json` (extends PLAN-Quality-Gates' schema-sync test coverage automatically via the reflection walk); `token_file` contents are trimmed and the file must be `0600` (warn otherwise); `role` is validated against `{hub, worker}`.

## Security model (applies to every phase)

- Workers and gateways authenticate to the hub with per-name tokens (revocable: `jeff hub token revoke <name>`); hub stores hashes only.
- The hub listener must sit behind TLS (reverse proxy or tailscale) — plain `:8443` binds to localhost by default; `--addr 0.0.0.0` prints a loud warning.
- Headless agents run with permissions skipped **by definition** — a worker machine is a blast-radius boundary. Document: give workers only the repos and credentials you'd give that agent; PLAN-Permission-Safety's config applies to *interactive* runs only.
- Chat input becomes prompt content, never shell content. The gateway must never eval/interpolate chat text into commands (the `send` frame carries it as data end-to-end; the inbox hook shellQuotes on injection — PLAN-Hooks-Hardening is the prerequisite that makes that true).
- Context packs can contain sensitive learnings → served only against a valid lease (Phase A edge case list).

## Open questions for the maintainer (decide before Phase A; defaults chosen so work can start)

1. **Where does the hub's gig store come from?** Default assumed: the hub machine's existing `GIG_HOME` becomes authoritative; laptops keep using `gig` CLI against their own stores for personal stuff. (Alternative: hub hosts a *separate* store per "workspace".)
2. **Upstream gig fixes:** decided — see `PLAN-Gig-Upgrades.md` (CAS claim, DSN pragmas, transactional events, ID-collision retry, event cursor). Hub serialization still stands even with CAS (the hub is also the event loop and offer router); the gig fixes are correctness for everyone else touching the store.
3. **Worker workspaces on task completion:** delete (current `jeff done` behavior) or keep N days for forensics? Default: delete, keep the log file.
4. **Single binary vs. build tags:** hub+worker+slack in the main `jeff` binary (default; ~1 new dep in A, 1 in B) or a separate `jeffd`? Default: same binary, subcommands.

## Suggested build order

```
PLAN-Phase1-Attrs-Resume ─┐
PLAN-Crew-Reliability ────┤
PLAN-Permission-Safety ───┼──►  Phase A (hub+worker) ──► Phase B (Slack) ──► Phase C (fleet)
PLAN-Pickup-Rollback* ────┤                                   │
PLAN-Agent-Providers ─────┘                                   └──► Phase D (memory loop) ──► Phase E (web UI)
(* with the task.Store interface amendment above)
```

Phase A is the investment; B is where it starts feeling like the vision; C/D are incremental. Each phase leaves the system fully usable.
