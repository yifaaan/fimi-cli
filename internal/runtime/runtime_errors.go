package runtime

import (
	"context"
	"errors"
	"fmt"
)

// formatToolFailureContent 格式化工具执行失败的内容，使其对模型可见。
func formatToolFailureContent(err *ToolExecutionError) string {
	failureKind := "error"
	if IsTemporary(err) {
		failureKind = "temporary"
	} else if IsRefused(err) {
		failureKind = "refused"
	}

	return fmt.Sprintf("tool execution failed (failure_kind: %s): %s", failureKind, err.Err.Error())
}

func isInterruptedError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// IsRetryable 判断某个错误是否允许 runtime 重新尝试当前 step。
// 这里约定：只要错误链里存在实现 Retryable() bool 的值，并返回 true，就视为可重试。
func IsRetryable(err error) bool {
	type retryable interface {
		Retryable() bool
	}

	var target retryable
	if !errors.As(err, &target) {
		return false
	}

	return target.Retryable()
}

// IsRefused 判断某个错误是否表示“请求被系统拒绝执行”。
// 这类错误通常发生在真正副作用发生之前，例如越界路径或无效参数。
func IsRefused(err error) bool {
	type refused interface {
		Refused() bool
	}

	var target refused
	if !errors.As(err, &target) {
		return false
	}

	return target.Refused()
}

// IsTemporary 判断某个错误是否表示“执行环境暂时失败”。
// temporary 只描述错误性质，不等价于 runtime 现在就应该自动重试。
func IsTemporary(err error) bool {
	type temporary interface {
		Temporary() bool
	}

	var target temporary
	if !errors.As(err, &target) {
		return false
	}

	return target.Temporary()
}
