package gateway

import (
	"log"
	"sync"
	"time"

	"github.com/slack-go/slack"
)

// ConversationMarker handles coordination and de-duplication of marking conversations as read
type ConversationMarker struct {
	conversationToReadCursorMap map[string]string

	sync.Mutex
}

// NewConversationMarker creates a new conversation marker
func NewConversationMarker() *ConversationMarker {
	return &ConversationMarker{
		conversationToReadCursorMap: map[string]string{},
	}
}

// Reset clears any pending conversation mark events.
func (cm *ConversationMarker) Reset() {
	cm.Lock()
	defer cm.Unlock()

	cm.conversationToReadCursorMap = make(map[string]string)
}

func (cm *ConversationMarker) markConversationDelayed(sc *slack.Client, conversationID, timestamp string) {
	time.Sleep(5 * time.Second)

	cm.Lock()
	shouldMark := cm.conversationToReadCursorMap[conversationID] == timestamp
	cm.Unlock()

	if shouldMark {
		err := sc.MarkConversation(conversationID, timestamp)
		if err != nil {
			log.Printf("error while marking conversation %v: %v", conversationID, err)
		}
	}
}

// MarkConversation prepares a conversation marker to be updated after a delay elapses.
// Subsequent calls within the delay period supercede prior calls to MarkConversation.
func (cm *ConversationMarker) MarkConversation(sc *slack.Client, conversationID, timestamp string) {
	cm.Lock()
	defer cm.Unlock()

	cm.conversationToReadCursorMap[conversationID] = timestamp
	go cm.markConversationDelayed(sc, conversationID, timestamp)
}
