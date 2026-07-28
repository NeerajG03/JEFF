---
name: pr-review
description: PR review checklist and common issues to flag — correctness, test quality, style, and safety.
---

# PR Review

Systematic approach to reviewing pull requests. Covers correctness,
test coverage, code style, and security concerns.

## Before Starting

1. **Understand the scope** — read the task/ticket and diff summary first
2. **Check CI** — if build or tests are red, flag before deeper review
3. **Check for description** — PR should say what and why, not just link a ticket

## Review Checklist

### Correctness

- Does the change handle the **happy path** and **error paths**?
- Are there **edge cases** (empty input, nil, zero values, overflow)?
- Does it **preserve backward compatibility**? If not, is the breaking change justified?
- Are there **race conditions**? Look for shared mutable state without sync.

### Tests

- Are there **tests for the new code**? Not just the happy path.
- Do tests use **table-driven style** (Go) or parameterized (other langs)?
- Are tests **deterministic**? No sleeps, no network calls, no time-dependent assertions.
- Do **existing tests still pass**? Look for removed/renamed tests.
- Is there a **test for the bug fix** that reproduces the issue?

### Code Style

- Does the code **follow project conventions**? Check imports, naming, error handling.
- Are **errors wrapped** with context (`fmt.Errorf("do thing: %w", err)`)?
- Are there **magic numbers or strings** that should be constants?
- Are **comments explaining WHY**, not what (the code already says what)?
- Is **unused code** removed, not just commented out?

### Safety & Security

- Are **inputs validated**? Never trust external input.
- Are **secrets/credentials** ever logged or committed? Flag immediately.
- Is **user data** handled correctly (PII, escaping, encoding)?
- Are **permissions checked** for every operation?

### Performance

- Are there **N+1 queries** in loops?
- Is **memory allocation** reasonable? No unnecessary copies of large structs.
- Are **contexts propagated** for cancellation/timeout?

## Common Failure Patterns

| Pattern | What to flag |
|---------|-------------|
| Silent error swallow | `err != nil` without return or `err` checked but logged only |
| Shadowed variable | `:=` in inner scope shadows outer. Run `go vet` |
| Copy of sync.Mutex | Pass `*sync.Mutex`, not `sync.Mutex` |
| `defer` inside loop | Resources accumulate until function returns |
| Time comparison with `==` | Use `≈` delta or `time.Before`/`After` |
| Hardcoded timeout | Should be configurable or context-derived |

## Worked Example

**Scenario:** Reviewing a PR that adds a new HTTP handler.

1. **Check handler signature** — does it accept `http.ResponseWriter, *http.Request`?
2. **Check error handling** — are 4xx/5xx responses returned for bad input?
3. **Check middleware** — is auth/logging middleware applied?
4. **Check tests** — are there tests for: valid request, missing params, bad JSON?
5. **Check for panics** — no nil pointer dereference from request body parsing.
6. **Rate limiting** — is there any protection against abuse?
7. **Approve or request changes** with specific, actionable feedback.
