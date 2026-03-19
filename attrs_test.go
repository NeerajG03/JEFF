package jeff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neerajg/gig"
)

func tempGigStore(t *testing.T) *gig.Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := gig.Open(dbPath, gig.WithPrefix("test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close(); os.Remove(dbPath) })
	return store
}

func TestEnsureAttrs(t *testing.T) {
	store := tempGigStore(t)

	err := EnsureAttrs(store)
	if err != nil {
		t.Fatalf("ensure attrs: %v", err)
	}

	// Verify both attrs exist.
	def, err := store.GetAttrDef(AttrRepos)
	if err != nil {
		t.Fatalf("get repos attr: %v", err)
	}
	if def.Type != gig.AttrObject {
		t.Errorf("expected object type, got %s", def.Type)
	}

	def, err = store.GetAttrDef(AttrWorktreeSetup)
	if err != nil {
		t.Fatalf("get worktree_setup attr: %v", err)
	}
	if def.Type != gig.AttrString {
		t.Errorf("expected string type, got %s", def.Type)
	}
}

func TestEnsureAttrsIdempotent(t *testing.T) {
	store := tempGigStore(t)

	EnsureAttrs(store)
	err := EnsureAttrs(store)
	if err != nil {
		t.Fatalf("second ensure should be idempotent: %v", err)
	}
}
