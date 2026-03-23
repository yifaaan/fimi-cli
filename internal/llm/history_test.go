package llm

import (
	"reflect"
	"testing"

	"fimi-cli/internal/contextstore"
)

func TestTextRecordToMessage(t *testing.T) {
	message, ok := textRecordToMessage(contextstore.NewAssistantTextRecord("answer"))
	if !ok {
		t.Fatalf("textRecordToMessage() ok = false, want true")
	}
	if message != (Message{
		Role:    RoleAssistant,
		Content: "answer",
	}) {
		t.Fatalf("textRecordToMessage() = %#v, want %#v", message, Message{
			Role:    RoleAssistant,
			Content: "answer",
		})
	}
}

func TestTextRecordToMessageSkipsSystemRecord(t *testing.T) {
	_, ok := textRecordToMessage(contextstore.NewSystemTextRecord("boot"))
	if ok {
		t.Fatalf("textRecordToMessage() ok = true, want false")
	}
}

func TestBuildHistoryMessagesKeepsRecentConversation(t *testing.T) {
	records := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("boot"),
		contextstore.NewUserTextRecord("first"),
		contextstore.NewAssistantTextRecord("first reply"),
		contextstore.NewUserTextRecord("second"),
		contextstore.NewAssistantTextRecord("second reply"),
		contextstore.NewUserTextRecord("third"),
		contextstore.NewAssistantTextRecord("third reply"),
	}

	got := buildHistoryMessages(records, 2)
	want := []Message{
		{Role: RoleUser, Content: "second"},
		{Role: RoleAssistant, Content: "second reply"},
		{Role: RoleUser, Content: "third"},
		{Role: RoleAssistant, Content: "third reply"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildHistoryMessages() = %#v, want %#v", got, want)
	}
}

func TestBuildHistoryMessagesDropsLeadingAssistantAtWindowBoundary(t *testing.T) {
	records := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("boot"),
		contextstore.NewUserTextRecord("first"),
		contextstore.NewAssistantTextRecord("first reply"),
		contextstore.NewUserTextRecord("second"),
		contextstore.NewAssistantTextRecord("second reply"),
	}

	got := buildHistoryMessages(records, 1)
	want := []Message{
		{Role: RoleUser, Content: "second"},
		{Role: RoleAssistant, Content: "second reply"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildHistoryMessages() = %#v, want %#v", got, want)
	}
}

func TestBuildHistoryMessagesReturnsEmptyWhenLimitNonPositive(t *testing.T) {
	got := buildHistoryMessages([]contextstore.TextRecord{
		contextstore.NewUserTextRecord("hello"),
	}, 0)
	if len(got) != 0 {
		t.Fatalf("len(buildHistoryMessages()) = %d, want 0", len(got))
	}
}
