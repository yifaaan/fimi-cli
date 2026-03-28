package tools

// ToolParametersSchema 返回指定工具名对应的 JSON Schema 参数定义。
// 放在 tools 包内是因为 schema 是工具自身的固有知识，不属于应用层装配逻辑。
func ToolParametersSchema(name string) map[string]any {
	switch name {
	case ToolAgent:
		return objectSchema(requiredProperties(
			schemaProperty("description", "string", "Short task description for the subagent."),
			schemaProperty("prompt", "string", "Detailed task prompt for the subagent."),
			schemaProperty("subagent_name", "string", "Declared subagent name to run."),
		))
	case ToolThink:
		return objectSchema(requiredProperties(
			schemaProperty("thought", "string", "Private reasoning note to log for the current step."),
		))
	case ToolSetTodoList:
		return map[string]any{
			"type": "object",
			"$defs": map[string]any{
				"Todo": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{
							"type":        "string",
							"description": "The title of the todo",
							"minLength":   1,
						},
						"status": map[string]any{
							"type":        "string",
							"description": "The status of the todo",
							"enum":        []string{"Pending", "In Progress", "Done"},
						},
					},
					"required": []string{"title", "status"},
				},
			},
			"properties": map[string]any{
				"todos": map[string]any{
					"type":        "array",
					"description": "The updated todo list",
					"items": map[string]any{
						"$ref": "#/$defs/Todo",
					},
				},
			},
			"required": []string{"todos"},
		}
	case ToolBash:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to run inside the workspace.",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (0 = default 120s, max 300s).",
					"minimum":     0,
					"maximum":     300,
				},
				"background": map[string]any{
					"type":        "boolean",
					"description": "Run in background and return task ID immediately.",
					"default":     false,
				},
				"task_id": map[string]any{
					"type":        "string",
					"description": "Query status of a background task by its ID.",
				},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		}
	case ToolSearchWeb:
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search query to run on the web.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of results to return.",
					"minimum":     1,
					"maximum":     20,
					"default":     5,
				},
				"include_content": map[string]any{
					"type":        "boolean",
					"description": "Include fetched page content when the backend can provide it.",
					"default":     false,
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		}
	case ToolReadFile:
		return objectSchema(requiredProperties(
			schemaProperty("path", "string", "Workspace-relative file path to read."),
		))
	case ToolGlob:
		return objectSchema(requiredProperties(
			schemaProperty("pattern", "string", "Glob pattern relative to the workspace root."),
		))
	case ToolGrep:
		return objectSchema(requiredProperties(
			schemaProperty("pattern", "string", "Regular expression to search for."),
			schemaProperty("path", "string", "Workspace-relative file or directory path to search."),
		))
	case ToolWriteFile:
		return objectSchema(requiredProperties(
			schemaProperty("path", "string", "Workspace-relative file path to write."),
			schemaProperty("content", "string", "Full file contents to write."),
		))
	case ToolReplaceFile:
		return objectSchema(requiredProperties(
			schemaProperty("path", "string", "Workspace-relative file path to edit."),
			schemaProperty("old", "string", "Exact text to replace."),
			schemaProperty("new", "string", "Replacement text."),
		))
	case ToolPatchFile:
		return objectSchema(requiredProperties(
			schemaProperty("path", "string", "Workspace-relative file path to patch."),
			schemaProperty("diff", "string", "Unified diff patch content."),
		))
	case ToolFetchURL:
		return objectSchema(requiredProperties(
			schemaProperty("url", "string", "HTTP or HTTPS URL to fetch."),
		))
	default:
		return map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": true,
		}
	}
}

// --- schema DSL helpers ---

type schemaEntry struct {
	name   string
	schema map[string]any
}

func requiredProperties(properties ...schemaEntry) []schemaEntry {
	return properties
}

func schemaProperty(name, typeName, description string) schemaEntry {
	return schemaEntry{
		name: name,
		schema: map[string]any{
			"type":        typeName,
			"description": description,
		},
	}
}

func objectSchema(entries []schemaEntry) map[string]any {
	properties := make(map[string]any, len(entries))
	required := make([]string, 0, len(entries))
	for _, entry := range entries {
		properties[entry.name] = entry.schema
		required = append(required, entry.name)
	}

	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}
