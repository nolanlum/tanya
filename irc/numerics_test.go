package irc

import (
	"reflect"
	"testing"
)

func TestNumericReply_ToMessage(t *testing.T) {
	type fields struct {
		Code   NumericCommand
		Target string
		Params []string
	}
	tests := []struct {
		name   string
		fields fields
		want   *Message
	}{
		{
			name: "welcome message",
			fields: fields{
				Code:   RPL_WELCOME,
				Target: "papika",
				Params: []string{"Welcome to the tanya Slack IRC gateway papika"},
			},
			want: &Message{
				Prefix: "tanya",
				Cmd:    NumericReplyCmd,
				Params: []string{"001", "papika", "Welcome to the tanya Slack IRC gateway papika"},
			},
		},
		{
			name: "unknown command",
			fields: fields{
				Code:   ERR_UNKNOWNCOMMAND,
				Target: "papika",
				Params: []string{"POOP", "Unknown command"},
			},
			want: &Message{
				Prefix: "tanya",
				Cmd:    NumericReplyCmd,
				Params: []string{"421", "papika", "POOP", "Unknown command"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bn := &NumericReply{
				Code:   tt.fields.Code,
				Target: tt.fields.Target,
				Params: tt.fields.Params,
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

func TestErrUnknownCommand(t *testing.T) {
	want := &NumericReply{
		Code:   ERR_UNKNOWNCOMMAND,
		Target: "papika",
		Params: []string{"POOP", "Unknown command"},
	}
	got := ErrUnknownCommand("papika", "POOP")

	if !reflect.DeepEqual(got, want) {
		t.Errorf("ErrUnknownCommand() = %v, want %v", got, want)
	}
}
