package jeff

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// stringSet extracts a []any of strings (from a decoded JSON array) into a
// sorted, comparable slice.
func stringSet(vals []any) []string {
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, v.(string))
	}
	sort.Strings(out)
	return out
}

// TestConfigSchemaMatchesCode parses schemas/jeff-config.json and asserts its
// enums and top-level properties match what the Config struct and provider
// registries actually produce, so schema drift fails CI instead of failing
// users' editors.
func TestConfigSchemaMatchesCode(t *testing.T) {
	data, err := os.ReadFile("schemas/jeff-config.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no properties map")
	}

	agentProp, ok := properties["agent"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no agent property")
	}
	agentEnum := stringSet(agentProp["enum"].([]any))
	wantAgent := AgentTool("").ValidNames()
	sort.Strings(wantAgent)
	if !reflect.DeepEqual(agentEnum, wantAgent) {
		t.Errorf("agent enum = %v, want %v", agentEnum, wantAgent)
	}

	ideProp, ok := properties["ide"].(map[string]any)
	if !ok {
		t.Fatalf("schema has no ide property")
	}
	ideEnum := stringSet(ideProp["enum"].([]any))
	wantIDE := IDE("").ValidNames()
	sort.Strings(wantIDE)
	if !reflect.DeepEqual(ideEnum, wantIDE) {
		t.Errorf("ide enum = %v, want %v", ideEnum, wantIDE)
	}

	// Every persisted field on Config must be represented in the schema.
	cfgType := reflect.TypeOf(Config{})
	for i := 0; i < cfgType.NumField(); i++ {
		field := cfgType.Field(i)
		tag := field.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if _, ok := properties[name]; !ok {
			t.Errorf("Config field %s (json:%q) missing from schemas/jeff-config.json properties", field.Name, name)
		}
	}
}
