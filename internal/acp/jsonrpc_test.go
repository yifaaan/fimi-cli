package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"fimi-cli/internal/config"
	"fimi-cli/internal/contextstore"
	"fimi-cli/internal/runtime"
	"fimi-cli/internal/ui"
)

type rpcEnvelope struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      any              `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *RPCError        `json:"error,omitempty"`
}

func TestFramedConnServeSendsSyncHandlerResponse(t *testing.T) {
	input := bytes.NewBufferString("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n")
	var out bytes.Buffer
	conn := NewFramedConn(input, &out)
	conn.Register("ping", func(id any, params json.RawMessage) (any, error) {
		return map[string]string{"pong": "ok"}, nil
	})

	if err := conn.Serve(context.Background()); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	var msg rpcEnvelope
	if err := json.Unmarshal(out.Bytes(), &msg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if msg.ID.(float64) != 1 {
		t.Fatalf("response ID = %#v, want 1", msg.ID)
	}
	if msg.Error != nil {
		t.Fatalf("response error = %#v, want nil", msg.Error)
	}
}

func TestFramedConnServeDoesNotAutoRespondForAsyncHandler(t *testing.T) {
	input := bytes.NewBufferString("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"prompt\"}\n")
	var out bytes.Buffer
	conn := NewFramedConn(input, &out)

	done := make(chan struct{})
	conn.RegisterAsync("prompt", func(id any, params json.RawMessage) (any, error) {
		go func() {
			_ = conn.SendResponse(id, map[string]string{"status": "done"})
			close(done)
		}()
		return nil, nil
	})

	if err := conn.Serve(context.Background()); err != nil {
		t.Fatalf("Serve() error = %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("async handler did not send response")
	}

	var msg rpcEnvelope
	if err := json.Unmarshal(out.Bytes(), &msg); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if msg.Error != nil {
		t.Fatalf("response error = %#v, want nil", msg.Error)
	}
}

func TestHandlePromptReturnsNilAndSendsAsyncResponse(t *testing.T) {
	server, out := newTestServer(t)
	workDir := t.TempDir()
	sess := createPersistedSession(t, workDir)
	server.sessions[sess.ID] = NewSession(sess, server.conn, config.Default().DefaultModel)

	runCalled := make(chan struct{}, 1)
	server.runFn = func(ctx context.Context, store contextstore.Context, input runtime.Input, visualize ui.VisualizeFunc) (runtime.Result, error) {
		runCalled <- struct{}{}
		return runtime.Result{Status: runtime.RunStatusFinished}, nil
	}

	params, err := json.Marshal(PromptParams{
		SessionID: sess.ID,
		Prompt:    []ContentBlock{{Type: "text", Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := server.handlePrompt("1", params)
	if err != nil {
		t.Fatalf("handlePrompt() error = %v", err)
	}
	if got != nil {
		t.Fatalf("handlePrompt() result = %#v, want nil", got)
	}

	select {
	case <-runCalled:
	case <-time.After(time.Second):
		t.Fatal("runFn was not called")
	}

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) == 0 {
		t.Fatal("no async response written")
	}

	var msg rpcEnvelope
	if err := json.Unmarshal(lines[len(lines)-1], &msg); err != nil {
		t.Fatalf("json.Unmarshal(response) error = %v", err)
	}
	if msg.Error != nil {
		t.Fatalf("async response error = %#v, want nil", msg.Error)
	}
}

func TestHandlePromptRejectsUnsupportedContentType(t *testing.T) {
	server, _ := newTestServer(t)
	workDir := t.TempDir()
	sess := createPersistedSession(t, workDir)
	server.sessions[sess.ID] = NewSession(sess, server.conn, config.Default().DefaultModel)

	params, err := json.Marshal(PromptParams{
		SessionID: sess.ID,
		Prompt:    []ContentBlock{{Type: "image", Text: "ignored"}},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := server.handlePrompt("1", params)
	if err == nil {
		t.Fatal("handlePrompt() error = nil, want non-nil")
	}
	if err.Error() != "unsupported prompt content type: image" {
		t.Fatalf("handlePrompt() error = %q, want %q", err.Error(), "unsupported prompt content type: image")
	}
	if got != nil {
		t.Fatalf("handlePrompt() result = %#v, want nil", got)
	}
}

func TestHandlePromptRejectsEmptyPrompt(t *testing.T) {
	server, _ := newTestServer(t)
	workDir := t.TempDir()
	sess := createPersistedSession(t, workDir)
	server.sessions[sess.ID] = NewSession(sess, server.conn, config.Default().DefaultModel)

	params, err := json.Marshal(PromptParams{SessionID: sess.ID})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := server.handlePrompt("1", params)
	if err == nil {
		t.Fatal("handlePrompt() error = nil, want non-nil")
	}
	if err.Error() != "prompt is empty" {
		t.Fatalf("handlePrompt() error = %q, want %q", err.Error(), "prompt is empty")
	}
	if got != nil {
		t.Fatalf("handlePrompt() result = %#v, want nil", got)
	}
}

func TestHandlePromptSendsAsyncError(t *testing.T) {
	server, out := newTestServer(t)
	workDir := t.TempDir()
	sess := createPersistedSession(t, workDir)
	server.sessions[sess.ID] = NewSession(sess, server.conn, config.Default().DefaultModel)

	wantErr := "prompt failed"
	server.runFn = func(ctx context.Context, store contextstore.Context, input runtime.Input, visualize ui.VisualizeFunc) (runtime.Result, error) {
		return runtime.Result{}, errors.New(wantErr)
	}

	params, err := json.Marshal(PromptParams{
		SessionID: sess.ID,
		Prompt:    []ContentBlock{{Type: "text", Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	got, err := server.handlePrompt("1", params)
	if err != nil {
		t.Fatalf("handlePrompt() error = %v", err)
	}
	if got != nil {
		t.Fatalf("handlePrompt() result = %#v, want nil", got)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if out.Len() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if out.Len() == 0 {
		t.Fatal("no async error written")
	}

	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	var msg rpcEnvelope
	if err := json.Unmarshal(lines[len(lines)-1], &msg); err != nil {
		t.Fatalf("json.Unmarshal(error response) error = %v", err)
	}
	if msg.Error == nil {
		t.Fatal("async error response missing error object")
	}
	if msg.Error.Message != wantErr {
		t.Fatalf("async error message = %q, want %q", msg.Error.Message, wantErr)
	}
}
