package jeff

import (
	"os"
	"path/filepath"
	"testing"
)

// sandboxHome isolates $HOME and clears $JEFF_HOME so home resolution never
// touches the developer's real install, and returns the fake $HOME.
func sandboxHome(t *testing.T) string {
	t.Helper()
	fake := t.TempDir()
	t.Setenv("HOME", fake)
	t.Setenv("USERPROFILE", fake) // windows
	t.Setenv(EnvHome, "")
	return fake
}

func TestResolveHomePrecedence(t *testing.T) {
	t.Run("default when nothing is set", func(t *testing.T) {
		fake := sandboxHome(t)
		home, source, err := ResolveHomeWithSource()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(fake, ".jeff"); home != want {
			t.Errorf("home = %q, want %q", home, want)
		}
		if source != HomeSourceDefault {
			t.Errorf("source = %q, want %q", source, HomeSourceDefault)
		}
	})

	t.Run("pointer beats default", func(t *testing.T) {
		sandboxHome(t)
		if err := WriteHomePointer("/somewhere/custom"); err != nil {
			t.Fatal(err)
		}
		home, source, err := ResolveHomeWithSource()
		if err != nil {
			t.Fatal(err)
		}
		if home != "/somewhere/custom" || source != HomeSourcePointer {
			t.Errorf("got (%q, %q), want (/somewhere/custom, pointer)", home, source)
		}
	})

	t.Run("env beats pointer", func(t *testing.T) {
		sandboxHome(t)
		if err := WriteHomePointer("/somewhere/custom"); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvHome, "/from/env")
		home, source, err := ResolveHomeWithSource()
		if err != nil {
			t.Fatal(err)
		}
		if home != "/from/env" || source != HomeSourceEnv {
			t.Errorf("got (%q, %q), want (/from/env, env)", home, source)
		}
	})

	t.Run("relative env is normalized to absolute", func(t *testing.T) {
		sandboxHome(t)
		dir := t.TempDir()
		t.Chdir(dir)
		t.Setenv(EnvHome, "relative-home")
		home, source, err := ResolveHomeWithSource()
		if err != nil {
			t.Fatal(err)
		}
		if !filepath.IsAbs(home) {
			t.Errorf("home = %q, want an absolute path — a relative $%s would re-anchor to whatever cwd is current", home, EnvHome)
		}
		if filepath.Base(home) != "relative-home" || source != HomeSourceEnv {
			t.Errorf("got (%q, %q), want (<cwd>/relative-home, env)", home, source)
		}
	})

	t.Run("blank env is ignored", func(t *testing.T) {
		sandboxHome(t)
		if err := WriteHomePointer("/somewhere/custom"); err != nil {
			t.Fatal(err)
		}
		t.Setenv(EnvHome, "   ")
		home, source, _ := ResolveHomeWithSource()
		if home != "/somewhere/custom" || source != HomeSourcePointer {
			t.Errorf("got (%q, %q), want the pointer to win over whitespace-only env", home, source)
		}
	})
}

// TestResolveHomeNeverWrites is the regression test for the pointer-clobber bug.
//
// The read path used to "self-heal" the pointer on every command
// (main.go: `_ = jeff.WriteHomePointer(home)`). Combined with env-wins
// precedence, one throwaway `JEFF_HOME=/tmp/x jeff status` permanently repointed
// ~/.config/jeff/home for every future shell — the most transient layer promoting
// itself to the most durable one, silently.
//
// Resolution is READ-ONLY. This test pins that: after resolving with an env
// override in play, the pointer file must be byte-identical, and resolving with no
// pointer at all must not create one.
func TestResolveHomeNeverWrites(t *testing.T) {
	t.Run("existing pointer is untouched by an env override", func(t *testing.T) {
		sandboxHome(t)
		if err := WriteHomePointer("/the/real/home"); err != nil {
			t.Fatal(err)
		}
		ptr, err := PointerPath()
		if err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(ptr)
		if err != nil {
			t.Fatal(err)
		}

		t.Setenv(EnvHome, "/tmp/throwaway-experiment")
		for i := 0; i < 3; i++ {
			if _, _, err := ResolveHomeWithSource(); err != nil {
				t.Fatal(err)
			}
		}

		after, err := os.ReadFile(ptr)
		if err != nil {
			t.Fatalf("pointer file disappeared: %v", err)
		}
		if string(after) != string(before) {
			t.Errorf("resolution rewrote the pointer file:\n before: %q\n after:  %q\nA transient $%s must never become the persistent default.",
				before, after, EnvHome)
		}
	})

	t.Run("no pointer is created when none exists", func(t *testing.T) {
		sandboxHome(t)
		t.Setenv(EnvHome, "/tmp/throwaway-experiment")
		if _, _, err := ResolveHomeWithSource(); err != nil {
			t.Fatal(err)
		}
		ptr, err := PointerPath()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(ptr); err == nil {
			t.Errorf("resolution created %s; only `jeff init` and `jeff home use` may write the pointer", ptr)
		}
	})
}

// TestSelectHomeForInit covers the write path: `jeff init` must land where the
// resolution chain says the home is, so it cannot disagree with the command that
// runs next (#82).
func TestSelectHomeForInit(t *testing.T) {
	t.Run("honors JEFF_HOME", func(t *testing.T) {
		sandboxHome(t)
		t.Setenv(EnvHome, "/requested/by/env")
		home, source, err := SelectHomeForInit(SelectHomeOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if home != "/requested/by/env" || source != HomeSourceEnv {
			t.Errorf("got (%q, %q), want (/requested/by/env, env) — init must not silently use its own default", home, source)
		}
	})

	t.Run("honors the pointer file", func(t *testing.T) {
		sandboxHome(t)
		if err := WriteHomePointer("/recorded/home"); err != nil {
			t.Fatal(err)
		}
		home, source, err := SelectHomeForInit(SelectHomeOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if home != "/recorded/home" || source != HomeSourcePointer {
			t.Errorf("got (%q, %q), want (/recorded/home, pointer)", home, source)
		}
	})

	t.Run("explicit --home outranks env", func(t *testing.T) {
		sandboxHome(t)
		t.Setenv(EnvHome, "/from/env")
		want := filepath.Join(t.TempDir(), "explicit")
		home, source, err := SelectHomeForInit(SelectHomeOpts{Explicit: want})
		if err != nil {
			t.Fatal(err)
		}
		if home != want || source != HomeSourceFlag {
			t.Errorf("got (%q, %q), want (%q, flag)", home, source, want)
		}
	})

	t.Run("--here outranks env", func(t *testing.T) {
		sandboxHome(t)
		t.Setenv(EnvHome, "/from/env")
		dir := t.TempDir()
		t.Chdir(dir)
		home, source, err := SelectHomeForInit(SelectHomeOpts{Here: true})
		if err != nil {
			t.Fatal(err)
		}
		// t.Chdir may hand back a symlinked path (/var vs /private/var on macOS);
		// compare basenames plus the jeff suffix rather than the full path.
		if filepath.Base(home) != "jeff" || source != HomeSourceFlag {
			t.Errorf("got (%q, %q), want (<cwd>/jeff, flag)", home, source)
		}
	})

	t.Run("falls back to the default", func(t *testing.T) {
		fake := sandboxHome(t)
		home, source, err := SelectHomeForInit(SelectHomeOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(fake, ".jeff"); home != want || source != HomeSourceDefault {
			t.Errorf("got (%q, %q), want (%q, default)", home, source, want)
		}
	})
}

func TestIsHomeInitialized(t *testing.T) {
	home := t.TempDir()
	if IsHomeInitialized(home) {
		t.Error("empty dir reported as initialized")
	}
	if err := os.WriteFile(ConfigPath(home), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsHomeInitialized(home) {
		t.Error("home with jeff.json reported as uninitialized")
	}
	if IsHomeInitialized("") {
		t.Error("empty path reported as initialized")
	}
}
