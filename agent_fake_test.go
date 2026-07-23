package jeff

import (
	"testing"
)

type fakeProvider struct{}

func (f *fakeProvider) Name() AgentTool                                         { return "fake" }
func (f *fakeProvider) Command() string                                         { return "fake-cli" }
func (f *fakeProvider) BuildLaunchArgs(opts LaunchOpts) []string                { return nil }
func (f *fakeProvider) BuildCurateArgs(prompt string, opts LaunchOpts) []string { return nil }
func (f *fakeProvider) SupportsInlinePrompt() bool                              { return false }
func (f *fakeProvider) ConfigDir() string                                       { return ".fake" }
func (f *fakeProvider) SkillsSubdir() string                                    { return "skills" }
func (f *fakeProvider) CommandsSubdir() string                                  { return "commands" }
func (f *fakeProvider) CommandFileExt() string                                  { return "md" }
func (f *fakeProvider) ContextFileAliases() []string                            { return nil }
func (f *fakeProvider) ContextFileName() string                                 { return "FAKE.md" }
func (f *fakeProvider) MemorySuppressEnv() map[string]string                    { return nil }
func (f *fakeProvider) SendTiming() SendTiming                                  { return SendTiming{} }
func (f *fakeProvider) OwnsModel(model string) bool                             { return model == "fake-model" }
func (f *fakeProvider) ModelExamples() []string                                 { return []string{"fake-model"} }
func (f *fakeProvider) DoctorDeps() []DoctorDep {
	return []DoctorDep{{Name: "fake-bin", Required: true}}
}
func (f *fakeProvider) EnsureHomeDirs(home string) error    { return nil }
func (f *fakeProvider) WriteHomeDefaults(home string) error { return nil }
func (f *fakeProvider) HookDeliveryKey() string             { return "fake" }

func TestProviderRegistryExtensibility(t *testing.T) {
	// 1. Register fake
	RegisterProvider(&fakeProvider{})
	defer func() {
		providersMu.Lock()
		delete(providers, "fake")
		providersMu.Unlock()
	}()

	// 2. Assert InferBackend works
	if InferBackend("fake-model") != "fake" {
		t.Errorf("InferBackend failed to pick up fake-model")
	}
}

func TestProviderRegistryDoctorDepsFake(t *testing.T) {
	// 1. Register fake
	RegisterProvider(&fakeProvider{})
	defer func() {
		providersMu.Lock()
		delete(providers, "fake")
		providersMu.Unlock()
	}()

	found := false
	for _, a := range RegisteredAgents() {
		if a == "fake" {
			found = true
		}
	}
	if !found {
		t.Errorf("RegisteredAgents didn't return fake")
	}
}
