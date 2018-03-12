package irc

import "testing"

func TestToMessage(t *testing.T) {
	msgStr := "hi im a poop"
	p := &Privmsg{
		From: "poop",
		Channel: "chatter-technical",
		Message: msgStr,
	}

	m := p.ToMessage()

	if m.Prefix != "poop!poop@localhost" {
		t.Error("Did not generate prefix corectly")
	}
	if m.Cmd != PrivmsgCmd {
		t.Error("Not setting command to PrivmsgCmd")
	}
	if len(m.Params) != 2 {
		t.Error("Not adding correct params to message")
	}
	if m.Params[0] != "#chatter-technical" {
		t.Error("Not setting channel name correctly")
	}
	if m.Params[1] != msgStr {
		t.Error("Not setting message string correctly")
	}
}

func TestToMessageEmptyPrefix(t *testing.T) {
	p := &Privmsg{
		From: "",
		Channel: "chatter-technical",
		Message: "hi im a poop",
	}

	m := p.ToMessage()

	if m.Prefix != "" {
		t.Error("Prefix is not empty when it should be")
	}
	if m.Cmd != PrivmsgCmd {
		t.Error("Not setting command to PrivmsgCmd")
	}
}
