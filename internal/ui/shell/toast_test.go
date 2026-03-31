package shell

import (
	"testing"
	"time"
)

func TestToastAdd(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{
		Level:   ToastInfo,
		Message: "hello",
	}})

	if len(m.toasts) != 1 {
		t.Fatalf("expected 1 toast, got %d", len(m.toasts))
	}
	if m.toasts[0].Message != "hello" {
		t.Errorf("expected message 'hello', got %q", m.toasts[0].Message)
	}
	if m.toasts[0].ID != 0 {
		t.Errorf("expected ID 0, got %d", m.toasts[0].ID)
	}
	if m.toasts[0].TTL != 5*time.Second {
		t.Errorf("expected default TTL 5s, got %v", m.toasts[0].TTL)
	}
	if m.toasts[0].CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestToastStackLimit(t *testing.T) {
	m := NewToastModel()
	for i := 0; i < 7; i++ {
		m, _ = m.Update(ToastAddMsg{Toast: Toast{
			Level:   ToastInfo,
			Message: "msg",
		}})
	}

	if len(m.toasts) != 5 {
		t.Fatalf("expected 5 toasts (max stack), got %d", len(m.toasts))
	}
	if m.toasts[0].ID != 2 {
		t.Errorf("expected first toast ID 2, got %d", m.toasts[0].ID)
	}
}

func TestToastDismiss(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{Level: ToastInfo, Message: "first"}})
	m, _ = m.Update(ToastAddMsg{Toast: Toast{Level: ToastInfo, Message: "second"}})

	m, _ = m.Update(ToastDismissMsg{ID: 0})
	if len(m.toasts) != 1 {
		t.Fatalf("expected 1 toast after dismiss, got %d", len(m.toasts))
	}
	if m.toasts[0].Message != "second" {
		t.Errorf("expected 'second' remaining, got %q", m.toasts[0].Message)
	}
}

func TestToastTickExpires(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{
		Level:     ToastInfo,
		Message:   "expires",
		CreatedAt: time.Now().Add(-6 * time.Second),
		TTL:       5 * time.Second,
	}})

	m, _ = m.Update(ToastTickMsg{Time: time.Now()})
	if len(m.toasts) != 0 {
		t.Fatalf("expected 0 toasts after expiry, got %d", len(m.toasts))
	}
}

func TestToastTickKeepsAlive(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{
		Level:     ToastInfo,
		Message:   "fresh",
		CreatedAt: time.Now(),
		TTL:       5 * time.Second,
	}})

	m, _ = m.Update(ToastTickMsg{Time: time.Now()})
	if len(m.toasts) != 1 {
		t.Fatalf("expected 1 toast still alive, got %d", len(m.toasts))
	}
}

func TestToastViewEmpty(t *testing.T) {
	m := NewToastModel()
	if v := m.View(); v != "" {
		t.Errorf("expected empty view, got %q", v)
	}
}

func TestToastViewNonEmpty(t *testing.T) {
	m := NewToastModel()
	m, _ = m.Update(ToastAddMsg{Toast: Toast{Level: ToastSuccess, Message: "done"}})
	v := m.View()
	if v == "" {
		t.Error("expected non-empty view")
	}
	if !contains(v, "done") {
		t.Errorf("expected view to contain 'done', got %q", v)
	}
}

func TestParseToastLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected ToastLevel
	}{
		{"info", ToastInfo},
		{"warning", ToastWarning},
		{"error", ToastError},
		{"success", ToastSuccess},
		{"unknown", ToastInfo},
		{"", ToastInfo},
	}
	for _, tt := range tests {
		got := parseToastLevel(tt.input)
		if got != tt.expected {
			t.Errorf("parseToastLevel(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
