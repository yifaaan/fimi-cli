package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fimi-cli/internal/approval"
	"fimi-cli/internal/runtime"
)

func newWriteFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeWriteFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		if a := approval.FromContext(ctx); a != nil {
			if err := a.Request(ctx, "write_file", args.Path); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetAbs, err := resolveWorkspacePath(rootAbs, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetRel, err := relativeWorkspacePath(rootAbs, targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		if err := os.MkdirAll(filepath.Dir(targetAbs), 0o755); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("create parent dir for %q: %w", targetAbs, err)
		}
		if err := os.WriteFile(targetAbs, []byte(args.Content), 0o644); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: fmt.Sprintf("wrote %d bytes to %s", len([]byte(args.Content)), targetRel),
		}, nil
	}
}

func newReplaceFileHandler(workDir string) HandlerFunc {
	return func(ctx context.Context, call runtime.ToolCall, definition Definition) (runtime.ToolExecution, error) {
		args, err := decodeReplaceFileArguments(call.Arguments)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		if a := approval.FromContext(ctx); a != nil {
			if err := a.Request(ctx, "replace_file", args.Path); err != nil {
				return runtime.ToolExecution{
					Call:   call,
					Output: "Tool execution rejected by user",
				}, nil
			}
		}

		rootAbs, err := resolveWorkspaceRoot(workDir)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetAbs, err := resolveWorkspacePath(rootAbs, args.Path)
		if err != nil {
			return runtime.ToolExecution{}, err
		}
		targetRel, err := relativeWorkspacePath(rootAbs, targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, err
		}

		data, err := os.ReadFile(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("read file %q for replace: %w", targetAbs, err)
		}
		info, err := os.Stat(targetAbs)
		if err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("stat file %q for replace: %w", targetAbs, err)
		}

		content := string(data)
		matchCount := strings.Count(content, args.Old)
		if matchCount == 0 {
			return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %s", ErrToolReplaceTargetMissing, targetRel))
		}

		var replaced string
		var outputMsg string
		if args.ReplaceAll {
			replaced = strings.ReplaceAll(content, args.Old, args.New)
			outputMsg = fmt.Sprintf("replaced %d occurrence(s) in %s", matchCount, targetRel)
		} else {
			if matchCount > 1 {
				return runtime.ToolExecution{}, markRefused(fmt.Errorf("%w: %s (found %d, set replace_all to true to replace all)", ErrToolReplaceTargetNotUnique, targetRel, matchCount))
			}
			replaced = strings.Replace(content, args.Old, args.New, 1)
			outputMsg = fmt.Sprintf("replaced 1 occurrence in %s", targetRel)
		}

		if err := os.WriteFile(targetAbs, []byte(replaced), info.Mode().Perm()); err != nil {
			return runtime.ToolExecution{}, fmt.Errorf("write replaced file %q: %w", targetAbs, err)
		}

		return runtime.ToolExecution{
			Call:   call,
			Output: outputMsg,
		}, nil
	}
}

func decodeWriteFileArguments(raw string) (writeFileArguments, error) {
	var args writeFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return writeFileArguments{}, markRefused(fmt.Errorf("%w: decode write_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return writeFileArguments{}, markRefused(ErrToolPathRequired)
	}

	return args, nil
}

func decodeReplaceFileArguments(raw string) (replaceFileArguments, error) {
	var args replaceFileArguments
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return replaceFileArguments{}, markRefused(fmt.Errorf("%w: decode replace_file arguments: %v", ErrToolArgumentsInvalid, err))
	}
	if strings.TrimSpace(args.Path) == "" {
		return replaceFileArguments{}, markRefused(ErrToolPathRequired)
	}
	if args.Old == "" {
		return replaceFileArguments{}, markRefused(ErrToolReplaceOldRequired)
	}

	return args, nil
}
