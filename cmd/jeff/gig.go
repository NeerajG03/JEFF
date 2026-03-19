package main

import (
	"fmt"

	"github.com/neerajg/gig"
)

// openGigStore opens the gig store using the configured or default gig home.
func openGigStore() (*gig.Store, error) {
	gigCfg, err := gig.LoadConfig("")
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
