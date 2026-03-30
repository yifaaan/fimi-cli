package dmail

import "testing"

func TestDenwaRenjiSendAndFetch(t *testing.T) {
	d := NewDenwaRenji()
	d.SetCheckpointCount(3)

	if err := d.Send(DMail{Message: "hello", CheckpointID: 1}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	msg, id, ok := d.Fetch()
	if !ok {
		t.Fatal("Fetch() ok = false, want true")
	}
	if msg != "hello" {
		t.Fatalf("Fetch() message = %q, want %q", msg, "hello")
	}
	if id != 1 {
		t.Fatalf("Fetch() checkpointID = %d, want %d", id, 1)
	}

	// Second fetch should return nothing
	_, _, ok = d.Fetch()
	if ok {
		t.Fatal("second Fetch() ok = true, want false")
	}
}

func TestDenwaRenjiRejectsDoubleSend(t *testing.T) {
	d := NewDenwaRenji()
	d.SetCheckpointCount(3)

	if err := d.Send(DMail{Message: "first", CheckpointID: 0}); err != nil {
		t.Fatalf("first Send() error = %v", err)
	}
	if err := d.Send(DMail{Message: "second", CheckpointID: 0}); err == nil {
		t.Fatal("second Send() should reject when a D-Mail is already pending")
	}
}

func TestDenwaRenjiRejectsInvalidCheckpoint(t *testing.T) {
	d := NewDenwaRenji()
	d.SetCheckpointCount(3)

	if err := d.Send(DMail{Message: "msg", CheckpointID: -1}); err == nil {
		t.Fatal("Send() should reject negative checkpoint_id")
	}
	if err := d.Send(DMail{Message: "msg", CheckpointID: 5}); err == nil {
		t.Fatal("Send() should reject checkpoint_id >= nCheckpoints")
	}
}

func TestDenwaRenjiFetchWhenEmpty(t *testing.T) {
	d := NewDenwaRenji()
	_, _, ok := d.Fetch()
	if ok {
		t.Fatal("Fetch() on empty should return false")
	}
}
