package gateway

import "testing"

func TestSlackClient_ParseMessageText(t *testing.T) {
	type fields struct {
		channelInfo map[string]*SlackChannel
		userInfo    map[string]*SlackUser
	}
	tests := []struct {
		name   string
		fields fields
		text   string
		want   string
	}{
		{
			name:   "normal message",
			fields: fields{},
			text:   "haha this is a message",
			want:   "haha this is a message",
		},
		{
			name:   "urlencode",
			fields: fields{},
			text:   "haha this is a &lt;message&gt; &amp; poop",
			want:   "haha this is a <message> & poop",
		},
		{
			name: "nick reference",
			fields: fields{
				userInfo: map[string]*SlackUser{
					"U2VEKS57B": {
						SlackID:  "U2VEKS57B",
						Nick:     "papika",
						RealName: "ピュア バリアー",
					},
				},
			},
			text: "le <@U2VEKS57B> face",
			want: "le @papika face",
		},
		{
			name:   "channel reference",
			fields: fields{},
			text:   "<#C2EFNRK1S|chatter-technical> is a cool place!",
			want:   "#chatter-technical is a cool place!",
		},
		{
			name:   "generic vanilla URL",
			fields: fields{},
			text:   "Fork this cool github repo <http://github.com/nolanlum/tanya> xD",
			want:   "Fork this cool github repo http://github.com/nolanlum/tanya xD",
		},
		{
			name:   "URL detected by slack",
			fields: fields{},
			text:   "Fork this cool github repo <http://github.com/nolanlum/tanya|github.com/nolanlum/tanya> xD",
			want:   "Fork this cool github repo github.com/nolanlum/tanya xD",
		},
		{
			name:   "mailto",
			fields: fields{},
			text:   "send cool pics of girls to <mailto:kedo@calanimagealpha.com|kedo@calanimagealpha.com> xD",
			want:   "send cool pics of girls to kedo@calanimagealpha.com xD",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := New()
			sc.channelInfo = tt.fields.channelInfo
			sc.userInfo = tt.fields.userInfo

			if got := sc.ParseMessageText(tt.text); got != tt.want {
				t.Errorf("SlackClient.ParseMessageText() = \"%v\", want \"%v\"", got, tt.want)
			}
		})
	}
}
