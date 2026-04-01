package tools

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"sync"
	"time"
)

// BackgroundTaskStatus 表示后台任务的当前状态。
type BackgroundTaskStatus string

const (
	BGStatusRunning  BackgroundTaskStatus = "running"
	BGStatusDone     BackgroundTaskStatus = "completed"
	BGStatusFailed   BackgroundTaskStatus = "failed"
	BGStatusTimedOut BackgroundTaskStatus = "timeout"
	BGStatusKilled   BackgroundTaskStatus = "killed"
)

// lockedBuffer 是一个线程安全的 byte buffer，
// 允许 goroutine 写入（作为 io.Writer）同时其他 goroutine 安全读取快照。
type lockedBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

// Snapshot 返回当前缓冲内容的副本。
func (b *lockedBuffer) Snapshot() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]byte, len(b.buf))
	copy(out, b.buf)
	return out
}

// backgroundTask 记录一个正在运行或已完成的后台进程。
type backgroundTask struct {
	id         string
	seq        int
	command    string
	status     BackgroundTaskStatus
	exitCode   int
	startedAt  time.Time
	finishedAt time.Time
	mu         sync.Mutex // 保护 status, exitCode, finishedAt

	cmd    *exec.Cmd
	cancel context.CancelFunc
	done   chan struct{} // 关闭表示进程已结束

	stdout *lockedBuffer
	stderr *lockedBuffer
}

// TaskResult 是查询后台任务状态的返回值。
type TaskResult struct {
	ID        string
	Command   string
	Status    BackgroundTaskStatus
	ExitCode  int
	Stdout    string
	Stderr    string
	StartedAt time.Time
	Duration  time.Duration
}

// BackgroundManager 管理当前会话中的所有后台 bash 进程。
// 每个会话拥有独立的 manager，进程间互不干扰。
type BackgroundManager struct {
	mu    sync.Mutex
	tasks map[string]*backgroundTask
	seq   int
}

// NewBackgroundManager 创建空的后台任务管理器。
func NewBackgroundManager() *BackgroundManager {
	return &BackgroundManager{
		tasks: make(map[string]*backgroundTask),
	}
}

// Start 在后台启动 bash 命令，立即返回任务 ID。
// timeout 为 0 表示使用默认后台超时（24h）。
func (m *BackgroundManager) Start(command string, workDir string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = 24 * time.Hour
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.seq++
	id := fmt.Sprintf("bg-%d", m.seq)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	cmd := exec.CommandContext(ctx, "bash", "-lc", command)
	if workDir != "" {
		cmd.Dir = workDir
	}

	task := &backgroundTask{
		id:        id,
		seq:       m.seq,
		command:   command,
		status:    BGStatusRunning,
		startedAt: time.Now(),
		cmd:       cmd,
		cancel:    cancel,
		done:      make(chan struct{}),
		stdout:    &lockedBuffer{},
		stderr:    &lockedBuffer{},
	}

	cmd.Stdout = task.stdout
	cmd.Stderr = task.stderr

	// 启动进程
	if err := cmd.Start(); err != nil {
		cancel()
		return "", fmt.Errorf("start background command: %w", err)
	}

	// 进程结束后异步更新状态
	go func() {
		defer close(task.done)
		defer cancel()

		waitErr := cmd.Wait()

		task.mu.Lock()
		defer task.mu.Unlock()

		task.finishedAt = time.Now()

		if task.status == BGStatusKilled || ctx.Err() == context.Canceled {
			task.status = BGStatusKilled
		} else if ctx.Err() == context.DeadlineExceeded {
			task.status = BGStatusTimedOut
		} else if waitErr != nil {
			task.status = BGStatusFailed
		} else {
			task.status = BGStatusDone
		}
		task.exitCode = exitCodeFromError(waitErr)
	}()

	m.tasks[id] = task
	return id, nil
}

// List 返回当前 manager 中所有后台任务的快照，按最新创建优先排序。
func (m *BackgroundManager) List() []TaskResult {
	m.mu.Lock()
	tasks := make([]*backgroundTask, 0, len(m.tasks))
	for _, task := range m.tasks {
		tasks = append(tasks, task)
	}
	m.mu.Unlock()

	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].seq > tasks[j].seq
	})

	results := make([]TaskResult, 0, len(tasks))
	for _, task := range tasks {
		results = append(results, snapshotBackgroundTask(task))
	}

	return results
}

// Status 返回指定任务的当前快照。
// 任务运行中也可以安全查询已捕获的输出。
func (m *BackgroundManager) Status(taskID string) (TaskResult, error) {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	m.mu.Unlock()

	if !ok {
		return TaskResult{}, fmt.Errorf("background task %q not found", taskID)
	}

	return snapshotBackgroundTask(task), nil
}

// Kill 终止指定的后台任务。
func (m *BackgroundManager) Kill(taskID string) error {
	m.mu.Lock()
	task, ok := m.tasks[taskID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("background task %q not found", taskID)
	}

	task.mu.Lock()
	defer task.mu.Unlock()

	if task.status != BGStatusRunning {
		return nil
	}

	task.cancel()
	task.status = BGStatusKilled
	return nil
}

// Close 终止所有运行中的后台任务。应在会话结束时调用。
func (m *BackgroundManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, task := range m.tasks {
		task.mu.Lock()
		if task.status == BGStatusRunning {
			task.cancel()
			task.status = BGStatusKilled
		}
		task.mu.Unlock()
	}
}

func snapshotBackgroundTask(task *backgroundTask) TaskResult {
	task.mu.Lock()
	defer task.mu.Unlock()

	result := TaskResult{
		ID:        task.id,
		Command:   task.command,
		Status:    task.status,
		ExitCode:  task.exitCode,
		Stdout:    string(task.stdout.Snapshot()),
		Stderr:    string(task.stderr.Snapshot()),
		StartedAt: task.startedAt,
	}

	if !task.finishedAt.IsZero() {
		result.Duration = task.finishedAt.Sub(task.startedAt)
	} else {
		result.Duration = time.Since(task.startedAt)
	}

	return result
}
