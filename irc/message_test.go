package irc

import (
	"reflect"
	"testing"
)

// TODO: Improve the testing here
func TestStringToMessageUser(t *testing.T) {
	msg, err := StringToMessage("USER szi szi irc.szi.indojin :Sasuga Za Indojin")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != UserCmd {
		t.Error("Could not parse a User command")
	}
}

func TestStringToMessageNick(t *testing.T) {
	msg, err := StringToMessage("NICK a")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != NickCmd {
		t.Error("Could not parse 'nick a' as Nick command")
	}
}

func TestStringToMessagePrivmsg(t *testing.T) {
	msg, err := StringToMessage("PRIVMSG #chatter hello")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != PrivmsgCmd {
		t.Error("Could not parse 'privmsg #chatter hello' as Privmsg command")
	}

	expectedParams := []string{"#chatter", "hello"}
	if !reflect.DeepEqual(msg.Params, expectedParams) {
		t.Errorf("Parsed message params = %v, wanted %v", msg.Params, expectedParams)
	}
}

func TestStringToMessagePrivmsgWithTrailing(t *testing.T) {
	msg, err := StringToMessage("PRIVMSG #chatter :hello this is a message")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != PrivmsgCmd {
		t.Error("Could not parse 'privmsg #chatter hello' as Privmsg command")
	}

	expectedParams := []string{"#chatter", "hello this is a message"}
	if !reflect.DeepEqual(msg.Params, expectedParams) {
		t.Errorf("Parsed message params = %v, wanted %v", msg.Params, expectedParams)
	}
}

func TestStringToMessageWithPrefix(t *testing.T) {
	msg, err := StringToMessage(":hello NICK a")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != NickCmd {
		t.Error("Could not parse ':hello NICK a' as Nick command")
	}
}

func TestStringToMessageErr(t *testing.T) {
	_, err := StringToMessage("atest blah")
	if err == nil {
		t.Error("'atest blah' should not be a valid message")
	}
}

func TestStringToMessageOnlyPrefixErr(t *testing.T) {
	_, err := StringToMessage(":hello")
	if err == nil {
		t.Error("':hello' should not be a valid message")
	}
}

func TestMessageToStringWithPrefix(t *testing.T) {
	expected := ":szi!szi@localhost PRIVMSG #chatter :hello this is a message"
	m := Message{
		Prefix: "szi!szi@localhost",
		Cmd:    PrivmsgCmd,
		Params: []string{"#chatter", "hello this is a message"},
	}
	s := m.String()
	if s != expected {
		t.Error(
			"Did not stringify prefixed message properly",
			"Got: [", s, "]",
			"Expected: [", expected, "]",
		)
	}
}

func TestMessageToStringNoPrefix(t *testing.T) {
	expected := "PRIVMSG #chatter :hello this is a messages"
	m := Message{
		Prefix: "",
		Cmd:    PrivmsgCmd,
		Params: []string{"#chatter", "hello this is a messages"},
	}
	s := m.String()
	if s != expected {
		t.Error(
			"Did not stringify prefixed message properly",
			"Got: [", s, "]",
			"Expected: [", expected, "]",
		)
	}
}
