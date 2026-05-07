package memory

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runProposeCmd builds a fresh cobra root, runs `propose <args...>`, and returns any error.
func runProposeCmd(t *testing.T, args []string) error {
	t.Helper()
	root := &cobra.Command{Use: "jeff"}
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.AddCommand(newProposeCmd())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"propose"}, args...))
	return root.Execute()
}

func TestProposeCmd_HappyPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	err := runProposeCmd(t, []string{
		"--name", "async-boundary",
		"--type", "feedback",
		"--description", "Use top-level error boundaries",
		"--body", "Don't wrap async in try/catch.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, "proposals", "jenko", "gig-test", "async-boundary.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected proposal at %s: %v", path, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read proposal: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "name: async-boundary") {
		t.Errorf("expected name in frontmatter, got:\n%s", content)
	}
	if !strings.Contains(content, "type: feedback") {
		t.Errorf("expected type in frontmatter, got:\n%s", content)
	}
}

func TestProposeCmd_PersonaTaskFromFlags(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	// No JEFF_PERSONA or JEFF_TASK_ID set — must use flags.

	err := runProposeCmd(t, []string{
		"--name", "flag-persona",
		"--type", "user",
		"--description", "desc",
		"--body", "body",
		"--persona", "schmidt",
		"--task", "gig-flag",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(home, "proposals", "schmidt", "gig-flag", "flag-persona.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected proposal at %s: %v", path, err)
	}
}

func TestProposeCmd_MissingPersona(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	// Ensure env vars are unset.
	t.Setenv("JEFF_PERSONA", "")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	err := runProposeCmd(t, []string{
		"--name", "test",
		"--type", "feedback",
		"--description", "d",
		"--body", "b",
	})
	if err == nil {
		t.Fatal("expected error for missing persona")
	}
	if !strings.Contains(err.Error(), "persona") {
		t.Errorf("expected persona mention in error, got: %v", err)
	}
}

func TestProposeCmd_MissingTask(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "")

	err := runProposeCmd(t, []string{
		"--name", "test",
		"--type", "feedback",
		"--description", "d",
		"--body", "b",
	})
	if err == nil {
		t.Fatal("expected error for missing task")
	}
	if !strings.Contains(err.Error(), "task") {
		t.Errorf("expected task mention in error, got: %v", err)
	}
}

func TestProposeCmd_InvalidType(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	err := runProposeCmd(t, []string{
		"--name", "test",
		"--type", "bogus",
		"--description", "d",
		"--body", "b",
	})
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("expected type name in error, got: %v", err)
	}
}

func TestProposeCmd_InvalidName_TooLong(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	longName := strings.Repeat("a", 65)
	err := runProposeCmd(t, []string{
		"--name", longName,
		"--type", "feedback",
		"--description", "d",
		"--body", "b",
	})
	if err == nil {
		t.Fatal("expected error for name > 64 chars")
	}
	if !strings.Contains(err.Error(), "64") {
		t.Errorf("expected 64 in error, got: %v", err)
	}
}

func TestProposeCmd_InvalidName_Slash(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	err := runProposeCmd(t, []string{
		"--name", "a/b",
		"--type", "feedback",
		"--description", "d",
		"--body", "b",
	})
	if err == nil {
		t.Fatal("expected error for name with '/'")
	}
}

func TestProposeCmd_InvalidName_NotKebab(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	for _, bad := range []string{"CamelCase", "UPPER", "-leading", "trailing-", "double--hyphen"} {
		err := runProposeCmd(t, []string{
			"--name", bad,
			"--type", "feedback",
			"--description", "d",
			"--body", "b",
		})
		if err == nil {
			t.Errorf("expected error for name %q", bad)
		}
	}
}

func TestProposeCmd_NameCollision(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	args := []string{
		"--name", "collision-test",
		"--type", "feedback",
		"--description", "d",
		"--body", "first",
	}

	// First write succeeds.
	if err := runProposeCmd(t, args); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Second write without --force fails.
	err := runProposeCmd(t, args)
	if err == nil {
		t.Fatal("expected collision error on second write")
	}
	if !strings.Contains(err.Error(), "force") {
		t.Errorf("expected --force hint in error, got: %v", err)
	}
}

func TestProposeCmd_NameCollision_Force(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	args := []string{
		"--name", "overwrite-test",
		"--type", "feedback",
		"--description", "d",
		"--body", "first",
	}
	if err := runProposeCmd(t, args); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	// Second write with --force overwrites.
	args2 := append(args[:len(args):len(args)], "--force")
	if err := runProposeCmd(t, args2); err != nil {
		t.Fatalf("forced overwrite failed: %v", err)
	}
}

// TestProposeCmd_PersonaRouting verifies that proposals land under the correct
// persona directory when JEFF_PERSONA is set — the bug that caused hardy's
// proposals to land under proposals/jeff/ instead of proposals/hardy/.
func TestProposeCmd_PersonaRouting(t *testing.T) {
	personas := []string{"hardy", "marlowe", "jenko", "jeff"}
	for _, persona := range personas {
		t.Run(persona, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("JEFF_HOME", home)
			t.Setenv("JEFF_PERSONA", persona)
			t.Setenv("JEFF_TASK_ID", "gig-5dcc")

			err := runProposeCmd(t, []string{
				"--name", "test-routing",
				"--type", "feedback",
				"--description", "routing test",
				"--body", "body",
			})
			if err != nil {
				t.Fatalf("persona %q: unexpected error: %v", persona, err)
			}

			want := filepath.Join(home, "proposals", persona, "gig-5dcc", "test-routing.md")
			if _, err := os.Stat(want); err != nil {
				t.Fatalf("persona %q: expected proposal at %s: %v", persona, want, err)
			}

			// Assert no proposal landed under any OTHER persona dir.
			for _, other := range personas {
				if other == persona {
					continue
				}
				wrong := filepath.Join(home, "proposals", other, "gig-5dcc", "test-routing.md")
				if _, err := os.Stat(wrong); err == nil {
					t.Errorf("persona %q: proposal incorrectly landed under %q dir: %s", persona, other, wrong)
				}
			}
		})
	}
}

func TestProposeCmd_AllTypes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("JEFF_HOME", home)
	t.Setenv("JEFF_PERSONA", "jenko")
	t.Setenv("JEFF_TASK_ID", "gig-test")

	for _, typ := range []string{"user", "feedback", "project", "reference"} {
		err := runProposeCmd(t, []string{
			"--name", "type-" + typ,
			"--type", typ,
			"--description", "d",
			"--body", "b",
		})
		if err != nil {
			t.Errorf("type %q: unexpected error: %v", typ, err)
		}
	}
}
