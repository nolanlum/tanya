package irc

import (
	"reflect"
	"testing"
)

func TestNumericReply_ToMessage(t *testing.T) {
	type fields struct {
		Code       NumericCommand
		Params     []string
		ServerName string
		Target     string
	}
	tests := []struct {
		name   string
		fields fields
		want   *Message
	}{
		{
			name: "welcome message",
			fields: fields{
				Code:       RPL_WELCOME,
				Params:     []string{"Welcome to the tanya Slack IRC gateway papika"},
				ServerName: "irc.tanya.isekai",
				Target:     "papika",
			},
			want: &Message{
				Prefix: "irc.tanya.isekai",
				Cmd:    NumericReplyCmd,
				Params: []string{"001", "papika", "Welcome to the tanya Slack IRC gateway papika"},
			},
		},
		{
			name: "unknown command",
			fields: fields{
				Code:       ERR_UNKNOWNCOMMAND,
				Params:     []string{"POOP", "Unknown command"},
				ServerName: "irc.tanya.isekai",
				Target:     "papika",
			},
			want: &Message{
				Prefix: "irc.tanya.isekai",
				Cmd:    NumericReplyCmd,
				Params: []string{"421", "papika", "POOP", "Unknown command"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bn := &NumericReply{
				Code:       tt.fields.Code,
				Params:     tt.fields.Params,
				ServerName: tt.fields.ServerName,
				Target:     tt.fields.Target,
			}
			if got := bn.ToMessage(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NumericReply.ToMessage() = \"%v\", want \"%v\"", got, tt.want)
			}
		})
	}
}

func TestNumericReply_Error(t *testing.T) {
	type fields struct {
		Code   NumericCommand
		Target string
		Params []string
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "unknown nick",
			fields: fields{
				Code:   ERR_NOSUCHNICK,
				Target: "papika",
				Params: []string{"cocona", "No such nick/channel"},
			},
			want: "401: \"cocona\",\"No such nick/channel\"",
		},
		{
			name: "unknown command",
			fields: fields{
				Code:   ERR_UNKNOWNCOMMAND,
				Target: "papika",
				Params: []string{"POOP", "Unknown command"},
			},
			want: "421: \"POOP\",\"Unknown command\"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &NumericReply{
				Code:   tt.fields.Code,
				Target: tt.fields.Target,
				Params: tt.fields.Params,
			}
			if got := n.Error(); got != tt.want {
				t.Errorf("NumericReply.Error() = \"%v\", want \"%v\"", got, tt.want)
			}
		})
	}
}

func TestWholistAsNumerics(t *testing.T) {
	users := []User{
		{
			Nick:     "SZI",
			Ident:    "~szi",
			Host:     "Sasuga.Za.Indojin",
			RealName: "haha im szi",
			Away:     false,
		},
	}
	channelName := "#indojins"
	serverName := "irc.indoj.in"
	target := "SZI"

	var got []string
	for _, numeric := range WholistAsNumerics(users, channelName, serverName) {
		numeric.ServerName = serverName
		numeric.Target = target
		got = append(got, numeric.ToMessage().String())
	}

	want := []string{
		":irc.indoj.in 352 SZI #indojins ~szi Sasuga.Za.Indojin irc.indoj.in SZI H :0 haha im szi",
		":irc.indoj.in 315 SZI #indojins :End of /WHO list",
	}

	if len(want) != len(got) {
		t.Errorf("len(WholistAsNumerics()) != len(want), %v != %v", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("WholistAsNumerics()[%v] = \n\"%v\"\n want \n\"%v\"", i, got[i], want[i])
		}

	}
}

func TestNamelistAsNumerics(t *testing.T) {
	users := []User{
		{
			Nick:     "SZI",
			Ident:    "~szi",
			Host:     "Sasuga.Za.Indojin",
			RealName: "haha im szi",
		},
		{
			Nick:     "acid`",
			Ident:    "~asid",
			Host:     "grass",
			RealName: "haha im asid xD",
		},
	}
	channelName := "#indojins"
	target := "SZI"

	var got []string
	for _, numeric := range NamelistAsNumerics(users, channelName) {
		numeric.ServerName = "irc.indoj.in"
		numeric.Target = target
		got = append(got, numeric.ToMessage().String())
	}

	want := []string{
		":irc.indoj.in 353 SZI = #indojins :SZI acid`",
		":irc.indoj.in 366 SZI #indojins :End of /NAMES list",
	}

	if len(want) != len(got) {
		t.Errorf("len(NamelistAsNumerics()) != len(want), %v != %v", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("NamelistAsNumerics()[%v] = \n\"%v\"\n want \n\"%v\"", i, got[i], want[i])
		}

	}
}

func TestWhoisAsNumerics(t *testing.T) {
	user := User{
		Nick:     "SZI",
		Ident:    "~szi",
		Host:     "Sasuga.Za.Indojin",
		RealName: "haha im szi",
		Away:     false,
	}
	serverName := "irc.indoj.in"
	target := "SZI"

	var got []string
	for _, numeric := range WhoisAsNumerics(user) {
		numeric.ServerName = serverName
		numeric.Target = target
		got = append(got, numeric.ToMessage().String())
	}

	want := []string{
		":irc.indoj.in 311 SZI SZI ~szi Sasuga.Za.Indojin * :haha im szi",
		":irc.indoj.in 312 SZI SZI tanya :Slack IRC Gateway",
		":irc.indoj.in 318 SZI SZI :End of /WHOIS list",
	}

	if len(want) != len(got) {
		t.Errorf("len(WhoisAsNumerics()) != len(want), %v != %v", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("WhoisAsNumerics()[%v] = \n\"%v\"\n want \n\"%v\"", i, got[i], want[i])
		}

	}
}

func TestErrUnknownCommand(t *testing.T) {
	want := &NumericReply{
		Code:   ERR_UNKNOWNCOMMAND,
		Params: []string{"POOP", "Unknown command"},
	}
	got := ErrUnknownCommand("POOP")

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ErrUnknownCommand() = %v, want %v", got, want)
	}
}
