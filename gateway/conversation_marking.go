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

func (cm *ConversationMarker) markConversationFunc(sc *slack.Client, conversationID string) func(string) {
	return func(timestamp string) {
		cm.conversationToReadCursorMap[conversationID] = timestamp

		go func() {
			time.Sleep(5 * time.Second)

			cm.Lock()
			defer cm.Unlock()
			if cm.conversationToReadCursorMap[conversationID] == timestamp {
				err := sc.MarkConversation(conversationID, timestamp)
				if err != nil {
					log.Printf("error while marking conversation %v: %v", conversationID, err)
				}
			}
		}()
	}
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

// MarkConversation prepares a conversation marker to be updated upon receipt of an ack for the given message ID
func (cm *ConversationMarker) MarkConversation(sc *slack.Client, conversationID string, messageID int) {
	cm.Lock()
	defer cm.Unlock()

	cm.rtmIDToMarkFuncMap[messageID] = cm.markConversationFunc(sc, conversationID)
}
