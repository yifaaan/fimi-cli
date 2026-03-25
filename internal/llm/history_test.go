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
	want := Message{
		Role:    RoleAssistant,
		Content: "answer",
	}
	if !reflect.DeepEqual(message, want) {
		t.Fatalf("textRecordToMessage() = %#v, want %#v", message, want)
	}
}

func TestTextRecordToMessageParsesAssistantToolCalls(t *testing.T) {
	record := contextstore.TextRecord{
		Role:          contextstore.RoleAssistant,
		Content:       "I will inspect the file.",
		ToolCallsJSON: `[{"ID":"call_read","Name":"read_file","Arguments":"{\"path\":\"main.go\"}"}]`,
	}

	message, ok := textRecordToMessage(record)
	if !ok {
		t.Fatalf("textRecordToMessage() ok = false, want true")
	}

	want := Message{
		Role:    RoleAssistant,
		Content: "I will inspect the file.",
		ToolCalls: []ToolCall{
			{
				ID:        "call_read",
				Name:      "read_file",
				Arguments: `{"path":"main.go"}`,
			},
		},
	}
	if !reflect.DeepEqual(message, want) {
		t.Fatalf("textRecordToMessage() = %#v, want %#v", message, want)
	}
}

func TestTextRecordToMessageSkipsSystemRecord(t *testing.T) {
	_, ok := textRecordToMessage(contextstore.NewSystemTextRecord("boot"))
	if ok {
		t.Fatalf("textRecordToMessage() ok = true, want false")
	}
}

// TestBuildHistoryMessagesKeepsRecentConversation 验证 turn limit 语义：
// 最后一个 user message 是"当前输入"，不计入 turn limit。
// limit=2 表示保留 2 个历史 turn，加上当前输入。
func TestBuildHistoryMessagesKeepsRecentConversation(t *testing.T) {
	records := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("boot"),
		contextstore.NewUserTextRecord("first"),
		contextstore.NewAssistantTextRecord("first reply"),
		contextstore.NewUserTextRecord("second"),
		contextstore.NewAssistantTextRecord("second reply"),
		contextstore.NewUserTextRecord("third"), // 当前输入
	}

	got := buildHistoryMessages(records, 2)
	// limit=2 保留 2 个历史 turn (first, second)，加上当前输入 (third)
	want := []Message{
		{Role: RoleUser, Content: "first"},
		{Role: RoleAssistant, Content: "first reply"},
		{Role: RoleUser, Content: "second"},
		{Role: RoleAssistant, Content: "second reply"},
		{Role: RoleUser, Content: "third"}, // 当前输入
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildHistoryMessages() = %#v, want %#v", got, want)
	}
}

// TestBuildHistoryMessagesDropsLeadingAssistantAtWindowBoundary 验证：
// 当最后一个 user message 是当前输入时，不参与 turn limit 计算。
func TestBuildHistoryMessagesDropsLeadingAssistantAtWindowBoundary(t *testing.T) {
	records := []contextstore.TextRecord{
		contextstore.NewSystemTextRecord("boot"),
		contextstore.NewUserTextRecord("first"),
		contextstore.NewAssistantTextRecord("first reply"),
		contextstore.NewUserTextRecord("second"), // 当前输入
	}

	got := buildHistoryMessages(records, 1)
	// limit=1 保留 1 个历史 turn (first)，加上当前输入 (second)
	want := []Message{
		{Role: RoleUser, Content: "first"},
		{Role: RoleAssistant, Content: "first reply"},
		{Role: RoleUser, Content: "second"}, // 当前输入
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

func TestBuildHistoryMessagesPreservesAssistantToolCallChain(t *testing.T) {
	records := []contextstore.TextRecord{
		contextstore.NewUserTextRecord("list current dir"),
		{
			Role:          contextstore.RoleAssistant,
			ToolCallsJSON: `[{"ID":"call_bash","Name":"bash","Arguments":"{\"command\":\"pwd && ls -la\"}"}]`,
		},
		contextstore.NewToolResultRecord("call_bash", "/tmp/project"),
		contextstore.NewUserTextRecord("summarize it"),
	}

	got := buildHistoryMessages(records, 2)
	want := []Message{
		{Role: RoleUser, Content: "list current dir"},
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{
					ID:        "call_bash",
					Name:      "bash",
					Arguments: `{"command":"pwd && ls -la"}`,
				},
			},
		},
		{Role: RoleTool, ToolCallID: "call_bash", Content: "/tmp/project"},
		{Role: RoleUser, Content: "summarize it"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildHistoryMessages() = %#v, want %#v", got, want)
	}
}
