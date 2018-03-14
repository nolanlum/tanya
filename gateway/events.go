package gateway

// SlackEventType represents a variant of SlackEvent
type SlackEventType int

// Constants corresponding to event types
const (
	MessageEvent SlackEventType = iota
	NickChangeEvent
	JoinEvent
	PartEvent
)

// A SlackEvent is an event from Slack that should be communicated
// to any connected IRC clients
type SlackEvent struct {
	EventType SlackEventType
	Data      interface{}
}

// MessageEventData represents a textual message which should
// be delivered to IRC clients
type MessageEventData struct {
	Nick    string
	Target  string
	Message string
}

// NickChangeEventData represents a Slack user changing their display name
type NickChangeEventData struct {
	OldNick string
	NewNick string
}
