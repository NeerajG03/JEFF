package jeff

import "fmt"

// AddOpenCodeModel registers a user-chosen --model alias (name) mapped to the
// real OpenCode provider/model id (actualModel). Errors on duplicate name or
// if either name or actualModel would be ambiguous with structural model
// recognition (see agent_opencode.go's OwnsModel).
func AddOpenCodeModel(cfg *Config, name, actualModel string) error {
	if name == "" {
		return fmt.Errorf("name must not be empty")
	}
	if !isOpenCodeModel(actualModel) {
		return fmt.Errorf("actual model %q must be a provider/model id", actualModel)
	}
	if isClaudeModel(name) || isGeminiModel(name) || isOpenCodeModel(name) {
		return fmt.Errorf("name %q collides with a reserved model alias/shape, choose a different name", name)
	}
	if cfg.OpenCodeModels == nil {
		cfg.OpenCodeModels = make(map[string]string)
	}
	if _, exists := cfg.OpenCodeModels[name]; exists {
		return fmt.Errorf("opencode model %q already registered", name)
	}

	cfg.OpenCodeModels[name] = actualModel
	if err := SaveConfig(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	SetOpenCodeModelAliases(cfg.OpenCodeModels)
	return nil
}

// RemoveOpenCodeModel unregisters an opencode model alias.
func RemoveOpenCodeModel(cfg *Config, name string) error {
	if _, exists := cfg.OpenCodeModels[name]; !exists {
		return fmt.Errorf("opencode model %q not registered", name)
	}

	delete(cfg.OpenCodeModels, name)
	if err := SaveConfig(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	SetOpenCodeModelAliases(cfg.OpenCodeModels)
	return nil
}

// ListOpenCodeModels returns a copy of the registered name -> actualModel map.
func ListOpenCodeModels(cfg *Config) map[string]string {
	out := make(map[string]string, len(cfg.OpenCodeModels))
	for name, actual := range cfg.OpenCodeModels {
		out[name] = actual
	}
	return out
}
