package irc

import "testing"

// TODO: Improve the testing here
func TestStringToMessageUser(t *testing.T) {
	msg, err := StringToMessage("user a")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != UserCmd {
		t.Error("Could not parse 'user a' as User command")
	}
}

func TestStringToMessageNick(t *testing.T) {
	msg, err := StringToMessage("nick a")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != NickCmd {
		t.Error("Could not parse 'nick a' as Nick command")
	}
}

func TestStringToMessagePrivmsg(t *testing.T) {
	msg, err := StringToMessage("privmsg #chatter hello")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != PrivmsgCmd {
		t.Error("Could not parse 'privmsg #chatter hello' as Privmsg command")
	}
}

func TestStringToMessageWithPrefix(t *testing.T) {
	msg, err := StringToMessage(":hello user a")
	if err != nil {
		t.Error(err)
	}
	if msg.Cmd != UserCmd {
		t.Error("Could not parse ':hello user a' as User command")
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
		t.Error("'atest blah' should not be a valid message")
	}
}

func TestMessageToStringWithPrefix(t *testing.T) {
	expected := ":szi!szi@localhost privmsg #chatter hello"
	m := Message{
		Prefix: "szi!szi@localhost",
		Cmd: PrivmsgCmd,
		Params: []string{"#chatter", "hello"},
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
	expected := "privmsg #chatter hello"
	m := Message{
		Prefix: "",
		Cmd: PrivmsgCmd,
		Params: []string{"#chatter", "hello"},
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
