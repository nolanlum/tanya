package irc

import "fmt"

// User represents a user identified by a nick and an ident
type User struct {
	Nick  string
	Ident string
	Host  string
}

func (u User) String() string {
	if len(u.Nick) > 0 && len(u.Ident) > 0 {
		if len(u.Host) > 0 {
			return fmt.Sprintf("%v!%v@%v", u.Nick, u.Ident, u.Host)
		} else {
			return fmt.Sprintf("%v!%v@localhost", u.Nick, u.Ident)
		}
	} else {
		return ""
	}
}

// Privmsg represents a line in an IRC conversation
type Privmsg struct {
	From    User
	Channel string
	Message string
}

// ToMessage turns a Privmsg into a Message
func (p *Privmsg) ToMessage() *Message {
	return &Message{
		p.From.String(),
		PrivmsgCmd,
		[]string{p.Channel, p.Message},
	}
}

// Nick represents a IRC user nick change event
type Nick struct {
	From    User
	NewNick string
}

// ToMessage turns a Nick into a Message
func (n *Nick) ToMessage() *Message {
	return &Message{
		n.From.String(),
		NickCmd,
		[]string{n.NewNick},
	}
}

// Pong is a PONG message
type Pong struct {
	Token string
}

// ToMessage turns a Pong into a Message
func (p *Pong) ToMessage() *Message {
	return &Message{
		"tanya",
		PongCmd,
		[]string{"tanya", p.Token},
	}
}
