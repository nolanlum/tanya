package irc

import (
	"errors"
	"strings"
)

// Command represents an IRC command
type Command int

// Constants corresponding to message types
const (
	NickCmd Command = iota
	UserCmd

	PrivmsgCmd
	JoinCmd
	PartCmd

	PingCmd
	PongCmd

	NumericReplyCmd
)

// Message corresponds to an IRC message
type Message struct {
	Prefix string
	Cmd    Command
	Params []string
}

var cmdToStrMap = map[Command]string{
	NickCmd: "NICK",
	UserCmd: "USER",

	PrivmsgCmd: "PRIVMSG",
	JoinCmd:    "JOIN",
	PartCmd:    "PART",

	PingCmd: "PING",
	PongCmd: "PONG",

	NumericReplyCmd: "",
}

func (m *Message) String() string {
	s, ok := cmdToStrMap[m.Cmd]
	if !ok {
		return ""
	}
	var b strings.Builder

	messageHasPrefix := m.Prefix != ""
	if messageHasPrefix {
		b.WriteString(":")
		b.WriteString(m.Prefix)
	}

	if s != "" {
		if messageHasPrefix {
			b.WriteString(" ")
		}
		b.WriteString(s)
	}

	for i, p := range m.Params {
		b.WriteString(" ")
		if i == len(m.Params)-1 && strings.ContainsRune(p, ' ') {
			b.WriteString(":")
		}
		b.WriteString(p)
	}
	return b.String()
}

// This is technically incorrect as the prefix must contain
// a valid nickname or servername, but that requires a stateful
// lookup so we will skip that in this check
func hasPrefix(str string) bool {
	if len(str) <= 0 {
		return false
	}
	return str[0] == ':'
}

// ErrMalformedIRCMessage represents the error returned when parsing an
// incoming IRC line fails due to invalid message syntax
var ErrMalformedIRCMessage = errors.New("malformed IRC message")

// StringToMessage takes a string with a line of input
// and returns a Message corresponding to the line
func StringToMessage(str string) (*Message, error) {
	splitStr := strings.Split(str, " ")

	var cmdStr string
	var prefix string
	var params []string
	var paramInd int
	if hasPrefix(str) {
		if len(splitStr) < 2 {
			return nil, ErrMalformedIRCMessage
		}
		prefix = strings.ToLower(splitStr[0])
		cmdStr = strings.ToUpper(splitStr[1])
		paramInd = 2
	} else {
		cmdStr = strings.ToUpper(splitStr[0])
		paramInd = 1
	}

	for i := paramInd; i < len(splitStr); i++ {
		if splitStr[i][0] == ':' {
			params = append(params, strings.Join(splitStr[i:], " ")[1:])
			break
		} else {
			params = append(params, splitStr[i])
		}
	}

	switch cmdStr {
	case "USER":
		if len(params) < 4 {
			return nil, ErrNeedMoreParams("", "USER")
		}
		return &Message{prefix, UserCmd, params}, nil
	case "NICK":
		if len(params) < 1 {
			return nil, ErrNeedMoreParams("", "NICK")
		}
		return &Message{prefix, NickCmd, params}, nil
	case "PRIVMSG":
		if len(params) < 1 {
			return nil, ErrNeedMoreParams("", "PRIVMSG")
		}
		return &Message{prefix, PrivmsgCmd, params}, nil
	case "JOIN":
		if len(params) < 1 {
			return nil, ErrNeedMoreParams("", "JOIN")
		}
		return &Message{prefix, JoinCmd, params}, nil
	case "PART":
		if len(params) < 1 {
			return nil, ErrNeedMoreParams("", "PART")
		}
		return &Message{prefix, PartCmd, params}, nil
	case "PING":
		return &Message{prefix, PingCmd, params}, nil
	default:
		return nil, ErrUnknownCommand("", cmdStr)
	}
}
