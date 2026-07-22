package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func Disabled(jeffHome string) bool {
	if v := os.Getenv("JEFF_MEMORY_DISABLE"); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	data, err := os.ReadFile(filepath.Join(jeffHome, "jeff.json"))
	if err != nil {
		return false
	}
	var cfg struct {
		Memory struct {
			Disabled bool `json:"disabled"`
		} `json:"memory"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}
	return cfg.Memory.Disabled
}
