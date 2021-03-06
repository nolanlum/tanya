package irc

import (
	"fmt"
	"strings"
)

// Messagable is a type that can be converted to an IRC message
type Messagable interface {
	ToMessage() *Message
}

// User represents a user identified by a nick and an ident
type User struct {
	Nick     string
	Ident    string
	Host     string
	RealName string

	Away bool
}

func (u User) String() string {
	if len(u.Nick) > 0 && len(u.Ident) > 0 {
		if len(u.Host) > 0 {
			return fmt.Sprintf("%v!%v@%v", u.Nick, u.Ident, u.Host)
		}
		return fmt.Sprintf("%v!%v@localhost", u.Nick, u.Ident)
	}
	return ""
}

// Privmsg represents a line in an IRC conversation
type Privmsg struct {
	From    User
	Target  string
	Message string
}

// ToMessage turns a Privmsg into a Message
func (p *Privmsg) ToMessage() *Message {
	return &Message{
		p.From.String(),
		PrivmsgCmd,
		[]string{p.Target, p.Message},
	}
}

// IsTargetChannel returns whether the target for this PrivMsg is a channel
func (p *Privmsg) IsTargetChannel() bool {
	return len(p.Target) > 0 && p.Target[0] == '#'
}

// IsValidTarget returns whether the private message target is legal or not
func (p *Privmsg) IsValidTarget() bool {
	return len(p.Target) > 0
}

// IsFromSelf returns whether the private message originated from self
func (p *Privmsg) IsFromSelf() bool {
	// if the from user is empty, then this is a self privmsg
	return p.From == User{}
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
	ServerName string
	Token      string
}

// ToMessage turns a Pong into a Message
func (p *Pong) ToMessage() *Message {
	return &Message{
		p.ServerName,
		PongCmd,
		[]string{p.ServerName, p.Token},
	}
}

// Join is a JOIN message
type Join struct {
	User    User
	Channel string
}

// ToMessage turns a Join into a Message
func (j *Join) ToMessage() *Message {
	return &Message{
		j.User.String(),
		JoinCmd,
		[]string{j.Channel},
	}
}

// Part is a PART message
type Part struct {
	User    User
	Channel string
	Message string
}

// ToMessage turns a Join into a Message
func (j *Part) ToMessage() *Message {
	return &Message{
		j.User.String(),
		PartCmd,
		[]string{j.Channel, j.Message},
	}
}

// Topic is a TOPIC message
type Topic struct {
	From    User
	Channel string
	Topic   string
}

// ToMessage turns a Topic into a Message
func (t *Topic) ToMessage() *Message {
	return &Message{
		t.From.String(),
		TopicCmd,
		[]string{t.Channel, t.Topic},
	}
}

// ParseUserString pares a string into an IRC User
func ParseUserString(s string) User {
	if s == "" {
		return User{}
	}

	splitForHost := strings.Split(s, "@")

	var host string
	if len(splitForHost) > 1 {
		host = splitForHost[1]
	}

	var nick string
	var ident string
	splitForIdent := strings.Split(splitForHost[0], "!")
	if len(splitForIdent) > 1 {
		ident = splitForIdent[1]
	}
	nick = splitForIdent[0]

	return User{
		Nick:  nick,
		Ident: ident,
		Host:  host,
	}
}

// ParseMessage parses an IRC Message into a higher-level IRC type
func ParseMessage(m *Message) (Messagable, error) {
	switch m.Cmd {
	case PrivmsgCmd:
		var target string
		var msg string
		if len(m.Params) == 1 {
			target = m.Params[0]
		} else if len(m.Params) > 1 {
			target = m.Params[0]
			msg = m.Params[1]
		}

		return &Privmsg{
			From:    ParseUserString(m.Prefix),
			Target:  target,
			Message: msg,
		}, nil
	default:
		return &Privmsg{}, fmt.Errorf("could not parse message")
	}
}
