package identity

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeIdentity writes a minimal valid identity file at <dir>/.jeff/orchestrator.json.
func writeProjectIdentity(t *testing.T, dir, id string) {
	t.Helper()
	path := ProjectFilePath(dir)
	if err := Write(path, &Identity{ID: id, Name: "n-" + id, CreatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("write identity at %s: %v", path, err)
	}
}

func detect(t *testing.T, env map[string]string, startDir, home, jeffHome string) (string, Source, error) {
	t.Helper()
	return detectWith(detectParams{
		getenv:   func(k string) string { return env[k] },
		startDir: startDir,
		home:     home,
		jeffHome: jeffHome,
	})
}

func TestDetect_EnvVarWinsOverFiles(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	writeProjectIdentity(t, dir, "from-file")

	id, src, err := detect(t, map[string]string{EnvVar: "from-env"}, dir, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if id != "from-env" || src != SourceEnv {
		t.Errorf("got (%q, %q), want (from-env, env)", id, src)
	}
}

func TestDetect_LegacyEnvVarHonored(t *testing.T) {
	id, src, err := detect(t, map[string]string{EnvVarLegacy: "jeff-DM20"}, t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if id != "jeff-DM20" || src != SourceEnv {
		t.Errorf("got (%q, %q), want (jeff-DM20, env)", id, src)
	}
}

func TestDetect_CWDFile(t *testing.T) {
	dir := t.TempDir()
	writeProjectIdentity(t, dir, "cwd-id")

	id, src, err := detect(t, nil, dir, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if id != "cwd-id" || src != SourceCWDFile {
		t.Errorf("got (%q, %q), want (cwd-id, cwd-file)", id, src)
	}
}

func TestDetect_ParentFile(t *testing.T) {
	home := t.TempDir()
	parent := filepath.Join(home, "proj")
	child := filepath.Join(parent, "sub", "deeper")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectIdentity(t, parent, "parent-id")

	id, src, err := detect(t, nil, child, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if id != "parent-id" || src != SourceParentFile {
		t.Errorf("got (%q, %q), want (parent-id, parent-file)", id, src)
	}
}

func TestDetect_CWDWinsOverParent(t *testing.T) {
	home := t.TempDir()
	parent := filepath.Join(home, "proj")
	child := filepath.Join(parent, "sub")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectIdentity(t, parent, "parent-id")
	writeProjectIdentity(t, child, "child-id")

	id, src, err := detect(t, nil, child, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if id != "child-id" || src != SourceCWDFile {
		t.Errorf("got (%q, %q), want (child-id, cwd-file)", id, src)
	}
}

// TestDetect_ParentWalkStopsAtHome ensures a file ABOVE $HOME is never picked
// up: the walk must terminate at $HOME rather than climbing to /.
func TestDetect_ParentWalkStopsAtHome(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home", "user")
	project := filepath.Join(home, "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	// Place an identity ABOVE home (at root). The walk must not reach it.
	writeProjectIdentity(t, root, "above-home")

	id, src, err := detect(t, nil, project, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if id != "" {
		t.Errorf("walk climbed above $HOME and found %q (via %q); want no match", id, src)
	}
}

// TestDetect_HomeFileMatched confirms an identity AT $HOME is still matched when
// $HOME is an ancestor of the start dir.
func TestDetect_HomeFileMatched(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "a", "b")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectIdentity(t, home, "home-id")

	id, src, err := detect(t, nil, project, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if id != "home-id" || src != SourceParentFile {
		t.Errorf("got (%q, %q), want (home-id, parent-file)", id, src)
	}
}

func TestDetect_GlobalFallback(t *testing.T) {
	home := t.TempDir()
	// Project dir on a path that is NOT under home (the real-world repro).
	project := t.TempDir()
	if err := Write(GlobalFilePath(home), &Identity{ID: "global-id", Name: "g", CreatedAt: "x"}); err != nil {
		t.Fatal(err)
	}

	id, src, err := detect(t, nil, project, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if id != "global-id" || src != SourceGlobalFile {
		t.Errorf("got (%q, %q), want (global-id, global-file)", id, src)
	}
}

func TestDetect_PerProjectWinsOverGlobal(t *testing.T) {
	home := t.TempDir()
	project := filepath.Join(home, "proj")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	writeProjectIdentity(t, project, "project-id")
	if err := Write(GlobalFilePath(home), &Identity{ID: "global-id", Name: "g", CreatedAt: "x"}); err != nil {
		t.Fatal(err)
	}

	id, _, err := detect(t, nil, project, home, home)
	if err != nil {
		t.Fatal(err)
	}
	if id != "project-id" {
		t.Errorf("got %q, want project-id (specificity wins)", id)
	}
}

func TestDetect_NoIdentityFound(t *testing.T) {
	id, src, err := detect(t, nil, t.TempDir(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatalf("expected nil err for not-found, got %v", err)
	}
	if id != "" || src != "" {
		t.Errorf("got (%q, %q), want empty", id, src)
	}
}

// TestDetect_MalformedFileFailsLoud is the crux of this change: a corrupt
// identity file must produce an error, NOT a silent fall-through to the global
// default or the shared orchestrator.
func TestDetect_MalformedFileFailsLoud(t *testing.T) {
	dir := t.TempDir()
	path := ProjectFilePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	id, _, err := detect(t, nil, dir, t.TempDir(), t.TempDir())
	if err == nil {
		t.Fatalf("malformed file returned id=%q, nil err; want error", id)
	}
}

func TestDetect_EmptyIDIsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := ProjectFilePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := detect(t, nil, dir, t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("identity file with empty id should be treated as malformed")
	}
}

func TestGenerate_Shape(t *testing.T) {
	fixed := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	id := Generate(GenerateOpts{Dir: "/x/y/my-project", Now: func() time.Time { return fixed }})
	if id.ID == "" {
		t.Error("id should be a generated UUID")
	}
	if id.Name != "my-project" {
		t.Errorf("name = %q, want my-project (basename of dir)", id.Name)
	}
	if id.CreatedAt != "2026-07-12T10:00:00Z" {
		t.Errorf("created_at = %q, want RFC3339 UTC", id.CreatedAt)
	}
	if id.TmuxPane != "" {
		t.Errorf("tmux_pane = %q, want empty (not in tmux)", id.TmuxPane)
	}
}

func TestGenerate_ExplicitNameAndPaneAndAdoptedID(t *testing.T) {
	id := Generate(GenerateOpts{ID: "jeff-DM20", Name: "custom", Dir: "/x/y", TmuxPane: "%53"})
	if id.ID != "jeff-DM20" {
		t.Errorf("id = %q, want adopted jeff-DM20", id.ID)
	}
	if id.Name != "custom" {
		t.Errorf("name = %q, want custom", id.Name)
	}
	if id.TmuxPane != "%53" {
		t.Errorf("tmux_pane = %q, want %%53", id.TmuxPane)
	}
}

// TestWrite_AtomicAndReadRoundTrip verifies Write persists a readable file and
// leaves no .tmp artifact behind.
func TestWrite_AtomicAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := ProjectFilePath(dir)
	want := &Identity{ID: "abc", Name: "proj", CreatedAt: "2026-07-12T00:00:00Z", TmuxPane: "%1"}
	if err := Write(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if *got != *want {
		t.Errorf("round-trip = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf(".tmp artifact left behind: %v", err)
	}
}

func TestWrite_OverwriteReplacesContent(t *testing.T) {
	dir := t.TempDir()
	path := ProjectFilePath(dir)
	if err := Write(path, &Identity{ID: "old", Name: "o", CreatedAt: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, &Identity{ID: "new", Name: "n", CreatedAt: "y"}); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "new" {
		t.Errorf("id = %q, want new", got.ID)
	}
}
