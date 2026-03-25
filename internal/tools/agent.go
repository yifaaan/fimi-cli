package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"fimi-cli/internal/runtime"
)

var ErrToolDescriptionRequired = errors.New("tool description is required")
var ErrToolPromptRequired = errors.New("tool prompt is required")
var ErrToolSubagentNameRequired = errors.New("tool subagent name is required")

type AgentArguments struct {
	Description  string `json:"description"`
	Prompt       string `json:"prompt"`
	SubagentName string `json:"subagent_name"`
}

func NewAgentToolHandler(
	run func(ctx context.Context, call runtime.ToolCall, args AgentArguments, definition Definition) (runtime.ToolExecution, error),
) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeAgentArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		return run(ctx, call, args, definition)
	}
}

func decodeAgentArguments(raw string) (AgentArguments, error) {
	var args AgentArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return AgentArguments{}, markRefused(fmt.Errorf("%w: decode agent arguments: %v", ErrToolArgumentsInvalid, err))
	}

	args.Description = strings.TrimSpace(args.Description)
	args.Prompt = strings.TrimSpace(args.Prompt)
	args.SubagentName = strings.TrimSpace(args.SubagentName)

	if args.Description == "" {
		return AgentArguments{}, markRefused(ErrToolDescriptionRequired)
	}
	if args.Prompt == "" {
		return AgentArguments{}, markRefused(ErrToolPromptRequired)
	}
	if args.SubagentName == "" {
		return AgentArguments{}, markRefused(ErrToolSubagentNameRequired)
	}

	return args, nil
}
