package wire

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

// ErrWireClosed is returned when receiving from a closed wire.
var ErrWireClosed = errors.New("wire is closed")

type currentWireKey struct{}

// Wire is a bidirectional channel between runtime and UI.
type Wire struct {
	ch           chan Message
	done         chan struct{}
	closed       atomic.Bool
	shutdownOnce sync.Once
}

// New creates a Wire with a buffered channel.
// If bufferSize <= 0, defaults to 64.
func New(bufferSize int) *Wire {
	if bufferSize <= 0 {
		bufferSize = 64
	}
	return &Wire{
		ch:   make(chan Message, bufferSize),
		done: make(chan struct{}),
	}
}

func WithCurrent(ctx context.Context, w *Wire) context.Context {
	return context.WithValue(ctx, currentWireKey{}, w)
}

func Current(ctx context.Context) (*Wire, bool) {
	w, ok := ctx.Value(currentWireKey{}).(*Wire)
	return w, ok && w != nil
}

// Send puts a message on the wire.
func (w *Wire) Send(msg Message) error {
	if w.closed.Load() {
		return ErrWireClosed
	}

	if req, ok := msg.(*ApprovalRequest); ok {
		req.mu.Lock()
		req.wireDone = w.done
		req.mu.Unlock()
	}

	select {
	case <-w.done:
		return ErrWireClosed
	case w.ch <- msg:
		return nil
	}
}

// Receive waits for the next message or returns error on shutdown.
// After shutdown, it drains any buffered messages before reporting closure.
func (w *Wire) Receive(ctx context.Context) (Message, error) {
	// Fast path: try non-blocking read from channel.
	select {
	case msg := <-w.ch:
		return msg, nil
	default:
	}

	// Slow path: wait for message, shutdown, or context cancellation.
	select {
	case msg := <-w.ch:
		return msg, nil
	case <-w.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Wire is shutting down or context is cancelled.
	// Drain any remaining buffered messages before reporting closure.
	select {
	case msg := <-w.ch:
		return msg, nil
	default:
		return nil, ErrWireClosed
	}
}

// Shutdown closes the wire. No more messages can be sent or received.
func (w *Wire) Shutdown() {
	w.shutdownOnce.Do(func() {
		w.closed.Store(true)
		close(w.done)
	})
}

// WaitForApproval sends an approval request and waits for the response.
// Returns ApprovalReject on context cancellation or wire shutdown.
func (w *Wire) WaitForApproval(ctx context.Context, req *ApprovalRequest) (ApprovalResponse, error) {
	if err := w.Send(req); err != nil {
		return ApprovalReject, err
	}
	return req.Wait(ctx)
}
