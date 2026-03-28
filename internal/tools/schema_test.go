package tools

import "testing"

func TestToolParametersSchemaForAgentTool(t *testing.T) {
	got := ToolParametersSchema(ToolAgent)

	properties, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got["properties"])
	}
	for _, name := range []string{"description", "prompt", "subagent_name"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("tool parameters missing %q property", name)
		}
	}
}

func TestToolParametersSchemaForThinkTool(t *testing.T) {
	got := ToolParametersSchema(ToolThink)

	properties, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got["properties"])
	}
	if _, ok := properties["thought"]; !ok {
		t.Fatalf("tool parameters missing %q property", "thought")
	}
}

func TestToolParametersSchemaForSetTodoListTool(t *testing.T) {
	got := ToolParametersSchema(ToolSetTodoList)

	defs, ok := got["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters $defs type = %T, want map[string]any", got["$defs"])
	}
	if _, ok := defs["Todo"]; !ok {
		t.Fatalf("tool parameters missing %q definition", "Todo")
	}
	properties, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got["properties"])
	}
	if _, ok := properties["todos"]; !ok {
		t.Fatalf("tool parameters missing %q property", "todos")
	}
}

func TestToolParametersSchemaForSearchWebTool(t *testing.T) {
	got := ToolParametersSchema(ToolSearchWeb)

	properties, ok := got["properties"].(map[string]any)
	if !ok {
		t.Fatalf("tool parameters properties type = %T, want map[string]any", got["properties"])
	}
	for _, name := range []string{"query", "limit", "include_content"} {
		if _, ok := properties[name]; !ok {
			t.Fatalf("tool parameters missing %q property", name)
		}
	}
	required, ok := got["required"].([]string)
	if !ok {
		t.Fatalf("tool parameters required type = %T, want []string", got["required"])
	}
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("tool parameters required = %#v, want []string{\"query\"}", required)
	}
}
