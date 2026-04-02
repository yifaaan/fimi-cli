package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
)

const (
	DefaultBashCommandTimeout = 120 * time.Second
	MaxBashCommandTimeout     = 300 * time.Second
)

var ErrToolCommandRequired = errors.New("tool command is required")
var ErrToolCommandTimedOut = errors.New("tool command timed out")

type bashArguments struct {
	Command    string `json:"command"`
	Timeout    int    `json:"timeout"`
	Background bool   `json:"background"`
	TaskID     string `json:"task_id"`
}

func newBashHandler(workDir string, shaper OutputShaper, bgMgr *BackgroundManager) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		if a := approval.FromContext(ctx); a != nil {
			desc := bashApprovalDescription(args)
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
			if err := a.Request(ctx, "bash", desc); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		if args.TaskID != "" {
			return handleBashTaskQuery(call, args, bgMgr)
		}
		if args.Background {
			return handleBashBackground(call, args, workDir, bgMgr)
		}
		return handleBashForeground(ctx, call, args, workDir, shaper)
	}
}

func handleBashTaskQuery(call runtime.ToolCall, args bashArguments, bgMgr *BackgroundManager) (runtime.ToolExecution, error) {
	if bgMgr == nil {
		return runtime.ToolExecution{}, markRefused(fmt.Errorf("background task manager not available"))
	}

	result, err := bgMgr.Status(args.TaskID)
	if err != nil {
		return runtime.ToolExecution{}, markRefused(err)
	}

	var outputParts []string
	outputParts = append(outputParts, fmt.Sprintf("Task %s [%s]", result.ID, result.Status))
	outputParts = append(outputParts, fmt.Sprintf("Command: %s", result.Command))
	outputParts = append(outputParts, fmt.Sprintf("Duration: %s", result.Duration.Round(time.Millisecond)))
	if result.ExitCode != 0 {
		outputParts = append(outputParts, fmt.Sprintf("Exit code: %d", result.ExitCode))
	}
	if result.Stdout != "" {
		outputParts = append(outputParts, "STDOUT:", result.Stdout)
	}
	if result.Stderr != "" {
		outputParts = append(outputParts, "STDERR:", result.Stderr)
	}

	joined := strings.Join(outputParts, "\n")
	return runtime.ToolExecution{
		Call:          call,
		Output:        joined,
		DisplayOutput: buildInlinePreview("", joined),
	}, nil
}

func handleBashBackground(call runtime.ToolCall, args bashArguments, workDir string, bgMgr *BackgroundManager) (runtime.ToolExecution, error) {
	if bgMgr == nil {
		return runtime.ToolExecution{}, markRefused(fmt.Errorf("background task manager not available"))
	}
	if strings.TrimSpace(args.Command) == "" {
		return runtime.ToolExecution{}, markRefused(ErrToolCommandRequired)
	}

	taskID, err := bgMgr.Start(args.Command, workDir, 0)
	if err != nil {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("start background task: %w", err))
	}

	message := fmt.Sprintf("Background task started: %s (use task_id=\"%s\" to check status)", taskID, taskID)
	return runtime.ToolExecution{
		Call:          call,
		Output:        message,
		DisplayOutput: buildInlinePreview("Ran "+args.Command, message),
	}, nil
}

func handleBashForeground(ctx context.Context, call runtime.ToolCall, args bashArguments, workDir string, shaper OutputShaper) (runtime.ToolExecution, error) {
	timeout := DefaultBashCommandTimeout
	if args.Timeout > 0 {
		timeout = min(time.Duration(args.Timeout)*time.Second, MaxBashCommandTimeout)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
	if strings.TrimSpace(workDir) != "" {
		cmd.Dir = workDir
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("%w: %s", ErrToolCommandTimedOut, args.Command))
	}
	if err != nil && !isExitError(err) {
		return runtime.ToolExecution{}, markTemporary(fmt.Errorf("run bash command: %w", err))
	}

	rawStdout := stdout.String()
	shapedStdout := shaper.Shape(rawStdout)
	rawStderr := stderr.String()
	shapedStderr := shaper.Shape(rawStderr)

	var outputParts []string
	outputParts = append(outputParts, shapedStdout.Output)
	if shapedStderr.Output != "" {
		outputParts = append(outputParts, "STDERR:", shapedStderr.Output)
	}

	var truncationMsgs []string
	if shapedStdout.Message != "" {
		truncationMsgs = append(truncationMsgs, "stdout: "+shapedStdout.Message)
	}
	if shapedStderr.Message != "" {
		truncationMsgs = append(truncationMsgs, "stderr: "+shapedStderr.Message)
	}
	if len(truncationMsgs) > 0 {
		outputParts = append(outputParts, "\n["+strings.Join(truncationMsgs, "; ")+"]")
	}

	return runtime.ToolExecution{
		Call:          call,
		Output:        strings.Join(outputParts, "\n"),
		DisplayOutput: buildSectionedPreview("Ran "+args.Command, previewSection{Label: "STDOUT", Content: rawStdout}, previewSection{Label: "STDERR", Content: rawStderr}),
		Stdout:        shapedStdout.Output,
		Stderr:        shapedStderr.Output,
		ExitCode:      exitCodeFromError(err),
	}, nil
}

func newBashHandlerWithTimeout(workDir string, shaper OutputShaper, timeout time.Duration) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeBashArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		cmd := exec.CommandContext(ctx, "bash", "-lc", args.Command)
		if workDir != "" {
			cmd.Dir = workDir
		}

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		err = cmd.Run()
		if err != nil {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return runtime.ToolExecution{}, markTemporary(fmt.Errorf("%w: %s", ErrToolCommandTimedOut, args.Command))
			}
			if !isExitError(err) {
				return runtime.ToolExecution{}, markTemporary(fmt.Errorf("run bash command: %w", err))
			}
		}

		shapedStdout := shaper.Shape(stdout.String())
		shapedStderr := shaper.Shape(stderr.String())
		return runtime.ToolExecution{
			Call:          call,
			Output:        shapedStdout.Output,
			DisplayOutput: buildSectionedPreview("Ran "+args.Command, previewSection{Label: "STDOUT", Content: stdout.String()}, previewSection{Label: "STDERR", Content: stderr.String()}),
			Stdout:        shapedStdout.Output,
			Stderr:        shapedStderr.Output,
			ExitCode:      exitCodeFromError(err),
		}, nil
	}
}

func decodeBashArguments(raw string) (bashArguments, error) {
	var args bashArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return bashArguments{}, markRefused(fmt.Errorf("%w: decode bash arguments: %v", ErrToolArgumentsInvalid, err))
	}

	args.Command = strings.TrimSpace(args.Command)
	args.TaskID = strings.TrimSpace(args.TaskID)
	if args.TaskID != "" {
		return args, nil
	}
	if args.Command == "" {
		return bashArguments{}, markRefused(ErrToolCommandRequired)
	}

	return args, nil
}

func bashApprovalDescription(args bashArguments) string {
	if args.TaskID != "" {
		return fmt.Sprintf("query background task %s", args.TaskID)
	}
	if args.Background {
		return "background: " + args.Command
	}
	return args.Command
}

func isExitError(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
