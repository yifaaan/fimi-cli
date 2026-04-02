package events

// RichContent preserves structured tool output for transports that can render
// more than plain text. Binary payloads should be base64-encoded in Data.
type RichContent struct {
	Type     string
	Text     string
	MIMEType string
	Data     string
}
