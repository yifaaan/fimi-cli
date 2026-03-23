package llm

import "fimi-cli/internal/contextstore"

func buildHistoryMessages(records []contextstore.TextRecord, limit int) []Message {
	if limit <= 0 {
		return []Message{}
	}

	messages := make([]Message, 0, len(records))
	turnCount := 0
	for _, record := range records {
		message, ok := textRecordToMessage(record)
		if !ok {
			continue
		}

		messages = append(messages, message)
		if message.Role == RoleUser {
			turnCount++
		}

		messages = dropLeadingNonUserMessages(messages)
		for turnCount > limit && len(messages) > 0 {
			if messages[0].Role == RoleUser {
				turnCount--
			}
			messages = messages[1:]
			messages = dropLeadingNonUserMessages(messages)
		}
	}

	return messages
}

func dropLeadingNonUserMessages(messages []Message) []Message {
	for len(messages) > 0 && messages[0].Role != RoleUser {
		messages = messages[1:]
	}

	return messages
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
