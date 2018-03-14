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

// Nick represents a IRC user nick change event
type Nick struct {
	From    string
	NewNick string
}

// ToMessage turns a Nick into a Message
func (n *Nick) ToMessage() *Message {
	var prefixStr string
	if n.From != "" {
		prefixStr = fmt.Sprintf("%v!%[1]v@localhost", n.From)
	}

	return &Message{
		prefixStr,
		NickCmd,
		[]string{n.NewNick},
	}
}
