package gateway

// SlackEventType represents a variant of SlackEvent
type SlackEventType int

// Constants corresponding to event types
const (
	SlackConnectedEvent SlackEventType = iota
	MessageEvent
	NickChangeEvent
	TopicChangeEvent
	SelfJoinEvent
	JoinEvent
	PartEvent
)

// A SlackEvent is an event from Slack that should be communicated
// to any connected IRC clients
type SlackEvent struct {
	EventType SlackEventType
	Data      interface{}
}

// SlackConnectedEventData represents the initial burst of data received upon
// establishment of a Slack RTM connection
type SlackConnectedEventData struct {
	UserInfo *SlackUser
}

// MessageEventData represents a textual message which should
// be delivered to IRC clients
type MessageEventData struct {
	From    SlackUser
	Target  string
	Message string
}

// NickChangeEventData represents a Slack user changing their display name
type NickChangeEventData struct {
	From    SlackUser
	NewNick string
}

// TopicChangeEventData represents a user changing the channel topic
type TopicChangeEventData struct {
	From     SlackUser
	Target   string
	NewTopic string
}

// JoinPartEventData represents a user joining or leaving a channel
type JoinPartEventData struct {
	User   SlackUser
	Target string
}
