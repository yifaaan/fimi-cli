package shell

import "time"

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
	PreviewRef string
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
		return b.Activity.Collapsible && b.Activity.Preview.Text != ""
	default:
		return false
	}
}
