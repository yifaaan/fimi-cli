package shell

import (
	"strings"
	"time"
)

type TranscriptBlockKind int

const (
	BlockKindUserPrompt TranscriptBlockKind = iota
	BlockKindAssistantNote
	BlockKindActivityGroup
	BlockKindApproval
	BlockKindDivider
	BlockKindElapsed
	BlockKindSystemNotice
	BlockKindError
)

type PreviewKind int

const (
	PreviewKindText PreviewKind = iota
	PreviewKindDiff
	PreviewKindMarkdown
)

type ActivityItemStatus int

const (
	ActivityItemPending ActivityItemStatus = iota
	ActivityItemRunning
	ActivityItemCompleted
	ActivityItemFailed
)

type ApprovalStatus int

const (
	ApprovalStatusPending ApprovalStatus = iota
	ApprovalStatusApproved
	ApprovalStatusApprovedForSession
	ApprovalStatusRejected
)

type PreviewBody struct {
	Text        string
	Kind        PreviewKind
	Collapsible bool
}

type ActivityItem struct {
	ToolCallID string
	Verb       string
	Text       string
	Status     ActivityItemStatus
	Preview    PreviewBody
}

type ActivityGroupBlock struct {
	GroupKind   string
	Title       string
	Items       []ActivityItem
	Preview     PreviewBody
	Accent      string
	Collapsible bool
}

type ApprovalBlock struct {
	RequestID   string
	Action      string
	Description string
	Selected    int
	Status      ApprovalStatus
}

type TranscriptBlock struct {
	ID        string
	Kind      TranscriptBlockKind
	CreatedAt time.Time

	UserText string
	NoteText string
	Activity ActivityGroupBlock
	Approval ApprovalBlock
	Text     string
}

func (b TranscriptBlock) IsCollapsible() bool {
	switch b.Kind {
	case BlockKindActivityGroup:
		if strings.TrimSpace(b.Activity.GroupKind) == "explored" && len(b.Activity.Items) > 0 {
			if strings.TrimSpace(b.Activity.Preview.Text) != "" {
				return true
			}
			for _, item := range b.Activity.Items {
				if strings.TrimSpace(item.Preview.Text) != "" {
					return true
				}
			}
			return false
		}
		if !b.Activity.Collapsible {
			return false
		}
		if strings.TrimSpace(b.Activity.Preview.Text) != "" {
			return true
		}
		for _, item := range b.Activity.Items {
			if strings.TrimSpace(item.Preview.Text) != "" {
				return true
			}
		}
		return false
	default:
		return false
	}
}
