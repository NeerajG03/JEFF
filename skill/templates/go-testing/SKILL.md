---
name: go-testing
description: Go testing patterns and conventions — table-driven tests, test helpers, mocks, and golden files.
---

# Go Testing

Standardised approach to writing Go tests in this codebase. Tests should be
readable, deterministic, and fast.

## Table-Driven Tests

Use sub-tests with a named struct slice:

```go
func TestParseDuration(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  time.Duration
        err   bool
    }{
        {name: "seconds", input: "30s", want: 30 * time.Second},
        {name: "invalid", input: "xyz", err: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := parseDuration(tt.input)
            if (err != nil) != tt.err {
                t.Fatalf("parseDuration(%q) err = %v, want err=%v", tt.input, err, tt.err)
            }
            if got != tt.want {
                t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
            }
        })
    }
}
```

## Test Helpers

Place shared helpers in `_test.go` files in a `testutil` package or in `export_test.go`
for white-box testing of unexported symbols:

```go
// export_test.go — exports internal symbols for testing.
package mypackage

var ParseConfig = parseConfig
```

Helper functions should call `t.Helper()`:

```go
func tempDir(t *testing.T) string {
    t.Helper()
    dir, err := os.MkdirTemp("", "test-*")
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { os.RemoveAll(dir) })
    return dir
}
```

## Mocking

Prefer hand-written mocks over mock generators. Keep mocks minimal — only
implement the interface methods used by the test.

```go
type mockStore struct {
    getFn func(id string) (Item, error)
}

func (m *mockStore) Get(id string) (Item, error) {
    return m.getFn(id)
}
```

## Golden Files

For complex output (JSON, HTML, generated code), use golden files:

```go
func TestRender(t *testing.T) {
    got := render(template, data)
    golden := filepath.Join("testdata", t.Name()+".golden")
    if *update {
        os.WriteFile(golden, []byte(got), 0o644)
    }
    want, _ := os.ReadFile(golden)
    if got != string(want) {
        t.Errorf("render mismatch. Update with -update")
    }
}
```

## Test Build Constraints

Use build tags for integration tests that need external services:

```go
// go:build integration

package mypackage_test

func TestDatabaseIntegration(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }
    // ...
}
```

## Worked Example

**Scenario:** Adding table-driven tests for a `ParseConfig` function that reads
YAML and returns a struct.

1. Create `parse_test.go` in the same package.
2. Define test cases covering: valid YAML, empty input, missing required field,
   unknown keys (error), and extended keys (no error if config allows).
3. Use `t.TempDir()` for config files, `t.Run()` per case.
4. Run `go test ./... -v -count=1` and verify all cases pass.
5. Run `go vet ./...` — no shadowed variables in test closures.
