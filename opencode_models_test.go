package jeff

import (
	"testing"

	"github.com/NeerajG03/JEFF/internal/testutil"
)

func TestAddOpenCodeModelRoundTrip(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}

	if err := AddOpenCodeModel(cfg, "fast", "opencode-go/kimi-k2.7-code"); err != nil {
		t.Fatalf("AddOpenCodeModel: %v", err)
	}

	loaded, err := LoadConfig(home)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	models := ListOpenCodeModels(loaded)
	if models["fast"] != "opencode-go/kimi-k2.7-code" {
		t.Errorf("ListOpenCodeModels after reload = %v, want fast -> opencode-go/kimi-k2.7-code", models)
	}
}

func TestAddOpenCodeModelDuplicate(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}

	if err := AddOpenCodeModel(cfg, "fast", "opencode-go/kimi-k2.7-code"); err != nil {
		t.Fatalf("AddOpenCodeModel: %v", err)
	}
	if err := AddOpenCodeModel(cfg, "fast", "opencode-go/kimi-k3"); err == nil {
		t.Error("expected error for duplicate opencode model name")
	}
}

func TestAddOpenCodeModelInvalidActual(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}

	if err := AddOpenCodeModel(cfg, "fast", "no-slash"); err == nil {
		t.Error("expected error for actual model without provider/model shape")
	}
}

func TestAddOpenCodeModelCollision(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}

	for _, name := range []string{"sonnet", "opus", "haiku", "claude-x", "pro", "flash", "flash-lite", "auto", "gemini-x", "provider/model"} {
		if err := AddOpenCodeModel(cfg, name, "opencode-go/kimi-k3"); err == nil {
			t.Errorf("expected collision error registering reserved-shaped name %q", name)
		}
	}
}

func TestRemoveOpenCodeModel(t *testing.T) {
	home := testutil.TempHome(t)
	cfg := &Config{Repos: make(map[string]*RepoConfig), Home: home}

	if err := AddOpenCodeModel(cfg, "fast", "opencode-go/kimi-k2.7-code"); err != nil {
		t.Fatalf("AddOpenCodeModel: %v", err)
	}
	if err := RemoveOpenCodeModel(cfg, "fast"); err != nil {
		t.Fatalf("RemoveOpenCodeModel: %v", err)
	}
	if err := RemoveOpenCodeModel(cfg, "fast"); err == nil {
		t.Error("expected error removing already-removed opencode model")
	}
}

func TestOwnsModelHybridFallback(t *testing.T) {
	SetOpenCodeModelAliases(nil)
	t.Cleanup(func() { SetOpenCodeModelAliases(nil) })

	p := GetProvider(AgentOpenCode)

	// Empty registry: permissive structural fallback.
	if !p.OwnsModel("opencode-go/kimi-k2.7-code") {
		t.Error("empty registry: expected structural provider/model to be recognized")
	}
	if p.OwnsModel("no-slash") {
		t.Error("empty registry: expected non-shaped model to be rejected")
	}

	SetOpenCodeModelAliases(map[string]string{"fast": "opencode-go/kimi-k2.7-code"})

	// Non-empty registry: hit by registered NAME.
	if !p.OwnsModel("fast") {
		t.Error("registered name should be recognized")
	}
	// Non-empty registry: hit by registered ACTUAL id.
	if !p.OwnsModel("opencode-go/kimi-k2.7-code") {
		t.Error("registered actual model id should be recognized")
	}
	// Non-empty registry: strict — structurally valid but unregistered is rejected.
	if p.OwnsModel("opencode-go/some-other-model") {
		t.Error("non-empty registry: expected unregistered provider/model to be rejected (strict mode)")
	}
}

func TestBuildLaunchArgsResolvesAlias(t *testing.T) {
	SetOpenCodeModelAliases(map[string]string{"fast": "opencode-go/kimi-k2.7-code"})
	t.Cleanup(func() { SetOpenCodeModelAliases(nil) })

	p := GetProvider(AgentOpenCode)
	args := p.BuildLaunchArgs(LaunchOpts{Model: "fast"})
	if len(args) != 2 || args[0] != "--model" || args[1] != "opencode-go/kimi-k2.7-code" {
		t.Errorf("BuildLaunchArgs with alias = %v, want [--model opencode-go/kimi-k2.7-code]", args)
	}

	// Passing the actual id directly still works unchanged.
	args = p.BuildLaunchArgs(LaunchOpts{Model: "opencode-go/kimi-k2.7-code"})
	if len(args) != 2 || args[0] != "--model" || args[1] != "opencode-go/kimi-k2.7-code" {
		t.Errorf("BuildLaunchArgs with actual id = %v, want [--model opencode-go/kimi-k2.7-code]", args)
	}
}
