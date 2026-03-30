package dmail

import (
	"errors"
	"sync"
)

// DMail represents a message sent back to a past checkpoint.
// The agent uses this to prune bloated context by reverting to
// an earlier state and injecting new information.
type DMail struct {
	Message      string `json:"message"`
	CheckpointID int    `json:"checkpoint_id"`
}

// DenwaRenji is the D-Mail state machine (named after the Phone Microwave
// from Steins;Gate). It holds a single pending D-Mail slot and tracks
// the current number of checkpoints for validation.
type DenwaRenji struct {
	mu           sync.Mutex
	pending      *DMail
	nCheckpoints int
}

// NewDenwaRenji creates a new DenwaRenji state machine.
func NewDenwaRenji() *DenwaRenji {
	return &DenwaRenji{}
}

// SetCheckpointCount updates the known number of checkpoints.
// Called after each checkpoint is created in the runtime loop.
func (d *DenwaRenji) SetCheckpointCount(n int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nCheckpoints = n
}

// Send stores a D-Mail for later retrieval by the runtime loop.
// Only one D-Mail can be pending at a time. The checkpoint must exist.
func (d *DenwaRenji) Send(mail DMail) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.pending != nil {
		return errors.New("a D-Mail is already pending; only one can be sent at a time")
	}
	if mail.CheckpointID < 0 {
		return errors.New("checkpoint_id must be non-negative")
	}
	if mail.CheckpointID >= d.nCheckpoints {
		return errors.New("checkpoint does not exist")
	}

	d.pending = &mail
	return nil
}

// Fetch retrieves and clears the pending D-Mail atomically.
// Returns (message, checkpointID, true) if a D-Mail was pending,
// or ("", 0, false) if none was pending.
func (d *DenwaRenji) Fetch() (string, int, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.pending == nil {
		return "", 0, false
	}

	mail := d.pending
	d.pending = nil
	return mail.Message, mail.CheckpointID, true
}
