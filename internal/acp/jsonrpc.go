package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Handler 处理一个 JSON-RPC 方法调用。
// id 是请求的标识符；params 是原始 JSON 参数。
// 返回 (result, error)。同步 handler 的 result 会由 Serve 自动发送；异步 handler 需要自己发送响应。
type Handler func(id any, params json.RawMessage) (any, error)

type registeredHandler struct {
	handler Handler
	async   bool
}

// FramedConn 管理 JSON-RPC 2.0 的 stdio 帧通信。
// 多个 goroutine 可以并发调用 Send* 方法；读循环在 Serve 中串行执行。
type FramedConn struct {
	reader   *bufio.Reader
	writer   io.Writer
	mu       sync.Mutex
	handlers map[string]registeredHandler
}

// NewFramedConn 创建一个新的 JSON-RPC 帧连接。
func NewFramedConn(r io.Reader, w io.Writer) *FramedConn {
	return &FramedConn{
		reader:   bufio.NewReader(r),
		writer:   w,
		handlers: make(map[string]registeredHandler),
	}
}

// Register 绑定同步方法名到处理函数。
func (c *FramedConn) Register(method string, handler Handler) {
	c.handlers[method] = registeredHandler{handler: handler}
}

// RegisterAsync 绑定异步方法名到处理函数。
// 该类 handler 需要自己调用 SendResponse 或 SendError。
func (c *FramedConn) RegisterAsync(method string, handler Handler) {
	c.handlers[method] = registeredHandler{handler: handler, async: true}
}

// SendResponse 发送一个 JSON-RPC 成功响应。
func (c *FramedConn) SendResponse(id any, result any) error {
	return c.writeJSON(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

// SendError 发送一个 JSON-RPC 错误响应。
func (c *FramedConn) SendError(id any, code int, message string) error {
	return c.writeJSON(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	})
}

// SendErrorWithData 发送一个带附加数据的 JSON-RPC 错误响应。
func (c *FramedConn) SendErrorWithData(id any, code int, message string, data any) error {
	return c.writeJSON(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message, Data: data},
	})
}

// SendNotification 发送一个 JSON-RPC 通知（没有 id）。
func (c *FramedConn) SendNotification(method string, params any) error {
	return c.writeJSON(Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

// Serve 启动读循环，解析每个 JSON 行并分发给注册的 handler。
// 阻塞直到读到 EOF、ctx 取消、或发生致命错误。
func (c *FramedConn) Serve(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := c.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read json-rpc frame: %w", err)
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			// 无法解析的请求，跳过（没有 id 无法发错误响应）
			continue
		}

		if req.JSONRPC != "2.0" {
			continue
		}

		registered, ok := c.handlers[req.Method]
		if !ok {
			if req.ID != nil {
				_ = c.SendError(req.ID, CodeMethodNotFound, "method not found: "+req.Method)
			}
			continue
		}

		result, err := registered.handler(req.ID, req.Params)
		if err != nil {
			if req.ID != nil {
				_ = c.SendError(req.ID, CodeInternalError, err.Error())
			}
			continue
		}

		if req.ID == nil || registered.async {
			continue
		}

		if err := c.SendResponse(req.ID, result); err != nil {
			return fmt.Errorf("send response: %w", err)
		}
	}
}

// writeJSON 序列化并写入一行 JSON，用 mutex 保护并发写。
func (c *FramedConn) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal json-rpc message: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	data = append(data, '\n')
	_, err = c.writer.Write(data)
	return err
}
