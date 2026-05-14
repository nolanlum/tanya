package gateway

import "sync"

// SentQueue handles de-deuplication of messages originating from this gateway echoed back to us by Slack.
type SentQueue struct {
	sentMessageSet map[sentMessage]struct{}

	sync.Mutex
}

type sentMessage struct {
	channel string
	ts      string
}

// NewSentQueue creates a new sent message tracker.
func NewSentQueue() *SentQueue {
	return &SentQueue{
		sentMessageSet: map[sentMessage]struct{}{},
	}
}

// Reset clears any pending message suppressions.
func (sq *SentQueue) Reset() {
	sq.Lock()
	defer sq.Unlock()

	sq.sentMessageSet = make(map[sentMessage]struct{})
}

// MessageSent adds a message (identified by ts) to be suppressed by a later call to ShouldSuppress.
func (sq *SentQueue) MessageSent(channelID, ts string) {
	sq.Lock()
	defer sq.Unlock()

	sq.sentMessageSet[sentMessage{channelID, ts}] = struct{}{}
}

// ShouldSuppress checks the set of sent messages to determine whether to suppress an own-message echo.
func (sq *SentQueue) ShouldSuppress(channelID, ts string) bool {
	sq.Lock()
	defer sq.Unlock()

	msg := sentMessage{channelID, ts}
	_, found := sq.sentMessageSet[msg]
	delete(sq.sentMessageSet, msg)
	return found
}
