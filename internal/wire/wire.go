package wire

import (
	"context"
	"errors"
)

// ErrWireClosed is returned when receiving from a closed wire.
var ErrWireClosed = errors.New("wire is closed")

// Wire is a bidirectional channel between runtime and UI.
type Wire struct {
	ch   chan Message  // main message channel (buffered)
	done chan struct{} // shutdown signal
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

// Send puts a message on the wire (non-blocking).
// Panics if wire is shut down.
func (w *Wire) Send(msg Message) {
	select {
	case w.ch <- msg:
	case <-w.done:
		panic("wire: send on closed wire")
	}
}

// Receive waits for the next message or returns error on shutdown.
func (w *Wire) Receive(ctx context.Context) (Message, error) {
	select {
	case msg := <-w.ch:
		return msg, nil
	case <-w.done:
		return nil, ErrWireClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Shutdown closes the wire. No more messages can be sent or received.
func (w *Wire) Shutdown() {
	close(w.done)
}
