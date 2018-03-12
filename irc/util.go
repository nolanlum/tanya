package irc

import "fmt"

// Utterance represents a line in an IRC conversation
type Utterance struct {
	From    string
	Channel string
	Message string
}

// ToMessage turns an Utterance into a Message
func (u *Utterance) ToMessage() *Message {
	var prefixStr string
	if u.From != "" {
		prefixStr = fmt.Sprintf("%v!%[1]v@localhost", u.From)	
	}
	
	channelStr := "#" + u.Channel
	return &Message{
		prefixStr,
		PrivmsgCmd,
		[]string{channelStr, u.Message},
	}
}
