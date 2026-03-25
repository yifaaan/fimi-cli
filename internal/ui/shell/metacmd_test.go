package shell

import (
	"testing"
)

func TestIsMetaCommandDetectsValidCommands(t *testing.T) {
	tests := []struct {
		input   string
		wantCmd string
		wantOk  bool
	}{
		{"/exit", "exit", true},
		{"/help", "help", true},
		{"/EXIT", "exit", true},  // case insensitive
		{"/Help", "help", true},
		{"exit", "", false},      // no slash prefix
		{"/unknown", "", false},  // unknown command
		{"", "", false},
		{"hello world", "", false},
	}

	for _, tt := range tests {
		gotCmd, gotOk := isMetaCommand(tt.input)
		if gotCmd != tt.wantCmd || gotOk != tt.wantOk {
			t.Errorf("isMetaCommand(%q) = (%q, %v), want (%q, %v)",
				tt.input, gotCmd, gotOk, tt.wantCmd, tt.wantOk)
		}
	}
}

func TestGetMetaCommandReturnsCorrectCommand(t *testing.T) {
	exitCmd, ok := getMetaCommand("exit")
	if !ok {
		t.Fatal("exit command not found")
	}
	if exitCmd.Name != "exit" {
		t.Errorf("exit.Name = %q, want %q", exitCmd.Name, "exit")
	}
	if exitCmd.Description == "" {
		t.Error("exit.Description is empty")
	}

	helpCmd, ok := getMetaCommand("help")
	if !ok {
		t.Fatal("help command not found")
	}
	if helpCmd.Name != "help" {
		t.Errorf("help.Name = %q, want %q", helpCmd.Name, "help")
	}
}
