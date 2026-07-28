---
name: root-cause
description: Root cause analysis workflow — systematic debugging, log analysis, hypothesis-driven investigation.
---

# Root Cause Analysis

Structured approach to debugging production issues and tracing root causes.
Replace guesswork with evidence-driven investigation.

## Investigation Workflow

### 1. Triage

- What is the **symptom**? (error message, alert, user report)
- What is the **blast radius**? (single user, all users, specific feature)
- When did it **start**? Correlate with deployments, config changes, or external events.
- Is it **reproducible**? If yes, get a minimal reproduction.

### 2. Gather Evidence

Start broad, then narrow:

- **Logs**: search for error codes, stack traces, correlated request IDs
- **Metrics**: latency spikes, error rate changes, saturation signals
- **Traces**: request path through services, slow spans, failed spans
- **Recent changes**: git log, deployment history, feature flag toggles

### 3. Form Hypotheses

Write down 2-3 possible root causes ranked by likelihood:

```
H1: Deploy of PR #1234 changed the API contract (high — timing matches)
H2: Database connection pool exhausted (medium — latency spike correlates)
H3: External API rate limit hit (low — no other services affected)
```

### 4. Test Each Hypothesis

Gather evidence that confirms or rules out each one:

- **H1**: Check diff of PR #1234. Roll back the change. Does the error stop? Yes → confirmed.
- **H2**: Check DB connection pool metrics. If pool size < active connections → confirmed.
- **H3**: Check external API response codes. If 429s → confirmed.

### 5. Fix and Verify

- Apply the fix (rollback, config change, code patch)
- Verify the symptom is resolved in production
- Add monitoring to detect recurrence

### 6. Document

- What was the root cause?
- What was the time to detect + time to fix?
- What would have caught it earlier? (missing test, missing alert, missing log)

## Log Analysis Patterns

| Finding | Likely Cause |
|---------|-------------|
| `context deadline exceeded` | Downstream service slow or hung |
| `connection refused` | Service not running, port wrong, firewall |
| `invalid memory address or nil pointer` | Missing nil check, uninitialised map/slice |
| `permission denied` | Wrong credentials, IAM role missing |
| `rate limit exceeded` | Caller too aggressive, quota too low |
| `no rows in result set` | Data not found, wrong query, missing migration |
| `duplicate key value` | Race on insert, missing unique constraint |

## Worked Example

**Scenario:** Users report 500 errors when saving campaign settings.
API logs show `ERROR: duplicate key value violates unique constraint "campaigns_slug_key"`.

**Triage:**
- Symptom: 500 on campaign save, constraint violation
- Blast radius: users creating campaigns with auto-generated slugs
- Start time: after deploy v2.3.1

**Evidence:**
- Git log: v2.3.1 changed slug generation from UUID to name-based
- Code: new slug = `strings.ToLower(campaign.Name)` — non-unique for common names
- Metrics: error rate for `POST /campaigns` went from 0.1% to 12%

**Hypothesis:**
H1: Name-based slug generation produces duplicates (high — code change matches error)

**Test:**
- Add logging for generated slugs → confirmed duplicates for "My Campaign"
- Fix: append random suffix to name-based slugs

**Document:**
- Root cause: slug generation changed from UUID to name without uniqueness check
- Missing: test for duplicate slug insertion
- Would have caught: integration test with two campaigns of the same name
