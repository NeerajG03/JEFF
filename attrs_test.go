package jeff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NeerajG03/gig"
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

	// Verify all 8 attrs exist with the expected type.
	wantTypes := map[string]gig.AttrType{
		AttrRepos:          gig.AttrObject,
		AttrWorktreeSetup:  gig.AttrString,
		AttrPersona:        gig.AttrString,
		AttrSkillsLoaded:   gig.AttrObject,
		AttrMemoryLoaded:   gig.AttrObject,
		AttrTeamSize:       gig.AttrString,
		AttrOutcome:        gig.AttrString,
		AttrRejectionCount: gig.AttrString,
	}
	for key, wantType := range wantTypes {
		def, err := store.GetAttrDef(key)
		if err != nil {
			t.Fatalf("get %s attr: %v", key, err)
		}
		if def.Type != wantType {
			t.Errorf("%s: expected type %s, got %s", key, wantType, def.Type)
		}
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
