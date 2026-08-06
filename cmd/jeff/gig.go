package main

import (
	"fmt"
	"os"
	"path/filepath"

	jeff "github.com/NeerajG03/JEFF"
	"github.com/NeerajG03/JEFF/workspace"
	"github.com/NeerajG03/gig"
)

// resolveGigHome returns the gig home directory using the documented precedence:
// GIG_HOME env → jeff.json gig_home → gig default (~/.gig).
func resolveGigHome(cfg *jeff.Config) string {
	if env := os.Getenv("GIG_HOME"); env != "" {
		return env
	}
	if cfg != nil && cfg.GigHome != "" {
		return cfg.GigHome
	}
	return gig.DefaultGigHome()
}

// gigTaskPrefix returns the task-ID prefix the gig store generates IDs with —
// the prefix every path-parsing site (taskctx, hook sync, worktree GC) must
// match against instead of a hardcoded "gig-" (#97).
//
// The source of truth is gig.yaml in the resolved gig home: that is the value
// openGigStore opens the store with, so it is what task IDs actually carry.
// jeff.json's gig_prefix is only the value recorded at init time and is
// consulted only when gig.yaml cannot be read.
func gigTaskPrefix(cfg *jeff.Config) string {
	gigHome := resolveGigHome(cfg)
	if gigCfg, err := gig.LoadConfig(filepath.Join(gigHome, "gig.yaml")); err == nil && gigCfg.Prefix != "" {
		return gigCfg.Prefix
	}
	if cfg != nil && cfg.GigPrefix != "" {
		return cfg.GigPrefix
	}
	return workspace.DefaultTaskIDPrefix
}

// isGigStoreInitialized returns true if gig.yaml exists in the resolved gig home.
func isGigStoreInitialized(cfg *jeff.Config) bool {
	gigHome := resolveGigHome(cfg)
	_, err := os.Stat(filepath.Join(gigHome, "gig.yaml"))
	return err == nil
}

// openGigStore opens the gig store using the configured or default gig home,
// respecting the same precedence as resolveGigHome.
func openGigStore(cfg *jeff.Config) (*gig.Store, error) {
	gigHome := resolveGigHome(cfg)
	gigCfg, err := gig.LoadConfig(filepath.Join(gigHome, "gig.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load gig config: %w", err)
	}

	store, err := gig.Open(gigCfg.DBPath,
		gig.WithPrefix(gigCfg.Prefix),
		gig.WithHashLength(gigCfg.HashLen),
		gig.WithConfig(gigCfg),
	)
	if err != nil {
		return nil, fmt.Errorf("open gig store: %w", err)
	}
	return store, nil
}
