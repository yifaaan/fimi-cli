package llm

import "fimi-cli/internal/contextstore"

// buildHistoryMessages 将历史记录转换为消息列表，同时应用 turn limit。
// 关键语义：最后一个 user message 是"当前输入"，不计入 turn limit。
// limit 限制的是"历史 turn"数量，而不是总 turn 数量。
func buildHistoryMessages(records []contextstore.TextRecord, limit int) []Message {
	if limit <= 0 {
		return []Message{}
	}

	// 第一遍：找出最后一个 user message 的位置
	// 这个位置之后的（包含）是"当前输入"，不计入 turn limit
	lastUserIdx := -1
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Role == contextstore.RoleUser {
			lastUserIdx = i
			break
		}
	}

	messages := make([]Message, 0, len(records))
	turnCount := 0
	for i, record := range records {
		message, ok := textRecordToMessage(record)
		if !ok {
			continue
		}

		messages = append(messages, message)
		// 只有历史 turn（非当前输入）才计入 turnCount
		isCurrentUserInput := i == lastUserIdx
		if message.Role == RoleUser && !isCurrentUserInput {
			turnCount++
		}

		messages = dropLeadingNonUserMessages(messages)
		for turnCount > limit && len(messages) > 0 {
			// 检查要丢弃的是不是当前输入
			// 如果是当前输入，不能丢弃，跳出循环
			isDroppingCurrentUserInput := len(messages) == 1 && messages[0].Role == RoleUser
			if isDroppingCurrentUserInput {
				break
			}
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
	case contextstore.RoleTool:
		// tool result 必须有 tool_call_id 才能关联回之前的调用
		if record.ToolCallID == "" {
			return Message{}, false
		}
		return Message{
			Role:       RoleTool,
			ToolCallID: record.ToolCallID,
			Content:    record.Content,
		}, true
	default:
		return Message{}, false
	}
}
