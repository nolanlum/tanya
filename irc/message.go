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
)

// Message corresponds to an IRC message
type Message struct {
	Cmd Command
	Params []string
}

var cmdToStrMap = map[Command]string{
	NickCmd: "nick",
	UserCmd: "user",
}

func (m *Message) String() string {
	s, ok := cmdToStrMap[m.Cmd]
	if !ok {
		return ""
	}
	return s
}

// StringToMessage takes a string with a line of input
// and returns a Message corresponding to the line
func StringToMessage(str string) (*Message, error) {
	splitStr := strings.Split(str, " ")
	if len(splitStr) < 1 {
		return nil, errors.New("malformed IRC message")
	}
	cmdStr := strings.ToLower(splitStr[0])
	switch(cmdStr) {
	case "user":
		return &Message{UserCmd, splitStr[1:]}, nil
	case "nick":
		return &Message{NickCmd, splitStr[1:]}, nil
	default:
		return nil, errors.New(splitStr[0] + " is not a valid IRC command")
	}
}
