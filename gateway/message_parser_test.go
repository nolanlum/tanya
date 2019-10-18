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
			text:   "Fork this cool github repo xD <http://github.com/nolanlum/tanya>",
			want:   "Fork this cool github repo xD http://github.com/nolanlum/tanya",
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
		{
			name:   "malformed",
			fields: fields{},
			text:   "le disconnect face <https://warosu.",
			want:   "le disconnect face https://warosu.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := NewSlackClient()
			sc.channelInfo = tt.fields.channelInfo
			sc.userInfo = tt.fields.userInfo

			if got := sc.ParseMessageText(tt.text); got != tt.want {
				t.Errorf("SlackClient.ParseMessageText() = \"%v\", want \"%v\"", got, tt.want)
			}
		})
	}
}

func TestSlackClient_ParseMessageTextWithOptions(t *testing.T) {
	type fields struct {
		channelInfo map[string]*SlackChannel
		userInfo    map[string]*SlackUser
	}
	type params struct {
		text                string
		includeCanonicalURL bool
	}
	tests := []struct {
		name   string
		fields fields
		params params
		want   string
	}{
		{
			name:   "URL detected by slack",
			fields: fields{},
			params: params{
				text:                "Fork this cool github repo <http://github.com/nolanlum/tanya|github.com/nolanlum/tanya> xD",
				includeCanonicalURL: true,
			},
			want: "Fork this cool github repo github.com/nolanlum/tanya http://github.com/nolanlum/tanya xD",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := NewSlackClient()
			sc.channelInfo = tt.fields.channelInfo
			sc.userInfo = tt.fields.userInfo

			if got := sc.ParseMessageTextWithOptions(tt.params.text, tt.params.includeCanonicalURL); got != tt.want {
				t.Errorf("SlackClient.ParseMessageTextWithOptions() = \"%v\", want \"%v\"", got, tt.want)
			}
		})
	}
}

func TestSlackClient_UnparseMessageText(t *testing.T) {
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
			text:   "haha this is a <message> & poop",
			want:   "haha this is a &lt;message&gt; &amp; poop",
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
			text: "le @papika face",
			want: "le <@U2VEKS57B> face",
		},
		{
			name: "nick reference with nbsp",
			fields: fields{
				userInfo: map[string]*SlackUser{
					"U267NCD1U": {
						SlackID:  "U267NCD1U",
						Nick:     "cozzie\u00a0alert",
						RealName: "Cozzie Kuns",
					},
				},
			},
			text: "le @cozzie\u00a0alert face",
			want: "le <@U267NCD1U> face",
		},
		{
			name: "IRC quote",
			fields: fields{
				userInfo: map[string]*SlackUser{
					"U267NCD1U": {
						SlackID:  "U267NCD1U",
						Nick:     "kedo",
						RealName: "Kenny Do",
					},
				},
			},
			text: "<@kedo> i like my girls like I like my boys... feminine",
			want: "&lt;<@U267NCD1U>&gt; i like my girls like I like my boys... feminine",
		},
		{
			name:   "IRC quote without user info",
			fields: fields{},
			text:   "<@kedo> i like my girls like I like my boys... feminine",
			want:   "&lt;@kedo&gt; i like my girls like I like my boys... feminine",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := NewSlackClient()
			sc.channelInfo = tt.fields.channelInfo
			sc.userInfo = tt.fields.userInfo
			sc.regenerateReverseMappings()

			if got := sc.UnparseMessageText(tt.text); got != tt.want {
				t.Errorf("SlackClient.UnparseMessageText() = \"%v\", want \"%v\"", got, tt.want)
			}
		})
	}
}
