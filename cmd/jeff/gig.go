package main

import (
	"fmt"
	"os"
	"path/filepath"

	jeff "github.com/NeerajG03/JEFF"
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
