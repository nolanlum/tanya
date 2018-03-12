package irc

import "fmt"

// Privmsg represents a line in an IRC conversation
type Privmsg struct {
	From    string
	Channel string
	Message string
}

// ToMessage turns a Privmsg into a Message
func (p *Privmsg) ToMessage() *Message {
	var prefixStr string
	if p.From != "" {
		prefixStr = fmt.Sprintf("%v!%[1]v@localhost", p.From)
	}

	return &Message{
		prefixStr,
		PrivmsgCmd,
		[]string{p.Channel, p.Message},
	}
}
