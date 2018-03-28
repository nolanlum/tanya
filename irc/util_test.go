package irc

import (
	"reflect"
	"testing"
)

func TestToMessage(t *testing.T) {
	msgStr := "hi im a poop"
	p := &Privmsg{
		From:    User{"poop", "poopser", "", "poop", false},
		Target:  "#chatter-technical",
		Message: msgStr,
	}

	m := p.ToMessage()

	if m.Prefix != "poop!poopser@localhost" {
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
		Target:  "chatter-technical",
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

func TestNick_ToMessage(t *testing.T) {
	type fields struct {
		From    User
		NewNick string
	}
	tests := []struct {
		name   string
		fields fields
		want   *Message
	}{
		{
			"no prefix",
			fields{User{}, "czi"},
			&Message{
				"",
				NickCmd,
				[]string{"czi"},
			},
		},
		{
			"with prefix",
			fields{User{"asid", "acid", "", "Asid Asid", false}, "czi"},
			&Message{
				"asid!acid@localhost",
				NickCmd,
				[]string{"czi"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Nick{
				From:    tt.fields.From,
				NewNick: tt.fields.NewNick,
			}
			if got := n.ToMessage(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Nick.ToMessage() = \"%v\", want \"%v\"", got, tt.want)
			}
		})
	}
}
