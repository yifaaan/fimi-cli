package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"fimi-cli/internal/dmail"
	"fimi-cli/internal/runtime"
)

// SendDMailArguments holds the parsed arguments for the send_dmail tool.
type SendDMailArguments struct {
	Message      string `json:"message"`
	CheckpointID int    `json:"checkpoint_id"`
}

// NewSendDMailHandler creates a handler for the send_dmail tool.
func NewSendDMailHandler(denwaRenji *dmail.DenwaRenji) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeSendDMailArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		mail := dmail.DMail{
			Message:      args.Message,
			CheckpointID: args.CheckpointID,
		}
		if err := denwaRenji.Send(mail); err != nil {
			return runtime.ToolExecution{
				Call:   call,
				Output: fmt.Sprintf("Failed to send D-Mail: %s", err),
			}, nil
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: "D-Mail sent. The context will be reverted to the target checkpoint.",
		}, nil
	}
}

func decodeSendDMailArguments(raw string) (SendDMailArguments, error) {
	var args SendDMailArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return SendDMailArguments{}, fmt.Errorf("decode send_dmail arguments: %w", err)
	}

	args.Message = strings.TrimSpace(args.Message)
	if args.Message == "" {
		return SendDMailArguments{}, fmt.Errorf("message is required")
	}

	return args, nil
}
