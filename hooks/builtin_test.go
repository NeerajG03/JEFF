package hooks

import "testing"

func TestBuiltinHooksGenerateContent(t *testing.T) {
	ctx := HookContext{
		JeffHome:  "/tmp/test-jeff",
		TargetDir: "/tmp/test-jeff",
		GigHome:   "/tmp/test-gig",
	}

	for _, h := range builtinHooks() {
		t.Run(h.Name+"/claude", func(t *testing.T) {
			if h.ClaudeScript == nil {
				t.Fatal("ClaudeScript is nil")
			}
			content := h.ClaudeScript(ctx)
			if content == "" {
				t.Fatal("ClaudeScript returned empty")
			}
			if len(content) < 20 {
				t.Fatalf("ClaudeScript suspiciously short: %q", content)
			}
		})

		t.Run(h.Name+"/opencode", func(t *testing.T) {
			if h.OpenCodeSnippet == nil {
				t.Fatal("OpenCodeSnippet is nil")
			}
			content := h.OpenCodeSnippet(ctx)
			if content == "" {
				t.Fatal("OpenCodeSnippet returned empty")
			}
		})
	}
}

func TestBuiltinHooksAreHomeSource(t *testing.T) {
	for _, h := range builtinHooks() {
		if h.Source != SourceHome {
			t.Errorf("%s: source = %q, want %q", h.Name, h.Source, SourceHome)
		}
	}
}

func TestBuiltinHooksAreSessionStart(t *testing.T) {
	for _, h := range builtinHooks() {
		if h.Event != "SessionStart" {
			t.Errorf("%s: event = %q, want SessionStart", h.Name, h.Event)
		}
	}
}

func TestTimeoutOrDefault(t *testing.T) {
	h := &Hook{Timeout: 0}
	if h.TimeoutOrDefault() != 10 {
		t.Fatalf("got %d, want 10", h.TimeoutOrDefault())
	}
	h.Timeout = 5
	if h.TimeoutOrDefault() != 5 {
		t.Fatalf("got %d, want 5", h.TimeoutOrDefault())
	}
}
