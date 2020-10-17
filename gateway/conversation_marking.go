package gateway

import (
	"log"
	"sync"
	"time"

	"github.com/slack-go/slack"
)

// ConversationMarker handles coordination and de-duplication of marking conversations as read
type ConversationMarker struct {
	rtmIDToMarkFuncMap          map[int]func(string)
	conversationToReadCursorMap map[string]string

	sync.Mutex
}

// NewConversationMarker creates a new conversation marker
func NewConversationMarker() *ConversationMarker {
	return &ConversationMarker{
		rtmIDToMarkFuncMap:          map[int]func(string){},
		conversationToReadCursorMap: map[string]string{},
	}
}

// Reset clears any pending conversation mark events.
func (cm *ConversationMarker) Reset() {
	cm.Lock()
	defer cm.Unlock()

	cm.rtmIDToMarkFuncMap = make(map[int]func(string))
	cm.conversationToReadCursorMap = make(map[string]string)
}

func (cm *ConversationMarker) markConversation(conversationID, timestamp string, mark func(string, string) error) {
	cm.conversationToReadCursorMap[conversationID] = timestamp

	go func() {
		time.Sleep(5 * time.Second)

		cm.Lock()
		defer cm.Unlock()
		if cm.conversationToReadCursorMap[conversationID] == timestamp {
			err := mark(conversationID, timestamp)
			if err != nil {
				log.Printf("error while marking conversation %v: %v", conversationID, err)
			}
		}
	}()
}

// HandleRTMAck handles an ACK from the RTM channel, scheduling a read marker update if possible
func (cm *ConversationMarker) HandleRTMAck(messageID int, timestamp string) {
	cm.Lock()
	defer cm.Unlock()

	markFunc, found := cm.rtmIDToMarkFuncMap[messageID]
	if found {
		delete(cm.rtmIDToMarkFuncMap, messageID)
		markFunc(timestamp)
	}
}

// MarkChannel prepares a channel marker to be updated upon receipt of an ack for the given message ID
func (cm *ConversationMarker) MarkChannel(sc *slack.Client, channelID string, messageID int) {
	cm.Lock()
	defer cm.Unlock()

	cm.rtmIDToMarkFuncMap[messageID] = func(timestamp string) {
		cm.markConversation(channelID, timestamp, sc.SetChannelReadMark)
	}
}

// MarkGroup prepares a group (private channel) marker to be updated upon receipt of an ack for the given message ID
func (cm *ConversationMarker) MarkGroup(sc *slack.Client, groupID string, messageID int) {
	cm.Lock()
	defer cm.Unlock()

	cm.rtmIDToMarkFuncMap[messageID] = func(timestamp string) {
		cm.markConversation(groupID, timestamp, sc.SetGroupReadMark)
	}
}

// MarkDM prepares a DM marker to be updated upon receipt of an ack for the given message ID
func (cm *ConversationMarker) MarkDM(sc *slack.Client, dmID string, messageID int) {
	cm.Lock()
	defer cm.Unlock()

	cm.rtmIDToMarkFuncMap[messageID] = func(timestamp string) {
		cm.markConversation(dmID, timestamp, sc.MarkIMChannel)
	}
}
