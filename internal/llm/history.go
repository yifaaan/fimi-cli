package llm

import "fimi-cli/internal/contextstore"

const historyMessageLimit = 2

func buildHistoryMessages(records []contextstore.TextRecord, limit int) []Message {
	messages := make([]Message, 0, len(records))
	for _, record := range records {
		message, ok := textRecordToMessage(record)
		if !ok {
			continue
		}
		messages = append(messages, message)
	}

	if len(messages) <= limit {
		return messages
	}

	// 先只截取最近几条对话历史，避免过早引入完整窗口裁剪策略。
	return messages[len(messages)-limit:]
}

func textRecordToMessage(record contextstore.TextRecord) (Message, bool) {
	switch record.Role {
	case contextstore.RoleUser:
		return Message{
			Role:    RoleUser,
			Content: record.Content,
		}, true
	case contextstore.RoleAssistant:
		return Message{
			Role:    RoleAssistant,
			Content: record.Content,
		}, true
	default:
		return Message{}, false
	}
}
