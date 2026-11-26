package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

type outputSchemaSample struct {
	Answer string           `json:"answer"`
	Data   any              `json:"data,omitempty"`
	Raw    json.RawMessage  `json:"raw,omitempty"`
	Rows   []any            `json:"rows,omitempty"`
	Nested outputSchemaNest `json:"nested"`
}

type outputSchemaNest struct {
	Variables any `json:"variables,omitempty"`
}

// A field that can hold anything reflects to the schema `true`. The MCP client
// library Claude Code is built on accepts only objects where a schema goes,
// and rejects the whole tool list over one `true`, so every tool disappears.
func TestOutputSchemaNeverUsesABooleanSchema(t *testing.T) {
	tool := NewTool("sample", WithOutputSchema[outputSchemaSample]())
	raw, err := json.Marshal(tool.OutputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if schema.Type != "object" || len(schema.Properties) != 5 {
		t.Fatalf("unexpected schema: %s", raw)
	}
	for name, prop := range schema.Properties {
		if _, ok := prop.(map[string]any); !ok {
			t.Errorf("property %q is %v, not an object schema", name, prop)
		}
	}
	// Nested schemas and array items are rewritten too.
	for _, bad := range []string{`"variables":true`, `"items":true`, `"data":true`, `"raw":true`} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("the schema still contains %s: %s", bad, raw)
		}
	}
	// A typed field keeps its type.
	if answer, _ := schema.Properties["answer"].(map[string]any); answer["type"] != "string" {
		t.Errorf("a typed field lost its type: %s", raw)
	}
}
