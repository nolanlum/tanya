package gateway

import (
	"fmt"
	"log"
	"strings"

	"github.com/nlopes/slack"
)

// SlackChannel holds data for a channel on Slack
type SlackChannel struct {
	SlackID string
	Name    string

	Topic slack.Topic
}

func slackChannelFromDto(channel *slack.Channel) *SlackChannel {
	return &SlackChannel{
		SlackID: channel.ID,
		Name:    "#" + channel.Name,
		Topic:   channel.Topic,
	}
}

// SlackUser holds data for each user on Slack
type SlackUser struct {
	SlackID  string
	Nick     string
	RealName string
}

func slackUserFromDto(user *slack.User) *SlackUser {
	// !sux slack
	nick := user.Profile.DisplayNameNormalized
	if nick == "" {
		nick = user.Profile.RealNameNormalized
	}

	return &SlackUser{
		SlackID:  user.ID,
		Nick:     nick,
		RealName: user.RealName,
	}
}

// SlackClient holds information for the websockets conn to Slack
type SlackClient struct {
	client *slack.Client
	rtm    *slack.RTM

	channelInfo map[string]*SlackChannel
	userInfo    map[string]*SlackUser

	slackUrlEncoder *strings.Replacer
	slackUrlDecoder *strings.Replacer
}

// NewSlackClient creates a new SlackClient with some default values
func NewSlackClient() *SlackClient {
	return &SlackClient{
		channelInfo: make(map[string]*SlackChannel),
		userInfo:    make(map[string]*SlackUser),

		slackUrlEncoder: strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;"),
		slackUrlDecoder: strings.NewReplacer("&gt;", ">", "&lt;", "<", "&amp;", "&"),
	}
}

// Make the initial Slack calls to bootstrap our connection
func (sc *SlackClient) bootstrapMappings() {
	hasMore := true
	gcp := &slack.GetConversationsParameters{
		ExcludeArchived: "true",
		Limit:           1000,
		Types:           []string{"public_channel", "private_channel"},
	}
	for hasMore {
		var channels []slack.Channel
		var err error
		channels, gcp.Cursor, err = sc.client.GetConversations(gcp)
		if err != nil {
			log.Println(err)
		}

		for _, channel := range channels {
			sc.channelInfo[channel.ID] = slackChannelFromDto(&channel)
		}

		hasMore = gcp.Cursor != ""
	}

	users, err := sc.client.GetUsers()
	if err != nil {
		log.Println(err)
	}
	for _, user := range users {
		sc.userInfo[user.ID] = slackUserFromDto(&user)
	}
}

// ResolveUser takes a slackID and fetches a SlackUser for the ID
func (sc *SlackClient) ResolveUser(slackID string) (user *SlackUser, err error) {
	user, found := sc.userInfo[slackID]
	if found {
		return
	}

	userInfo, err := sc.client.GetUserInfo(slackID)
	if err != nil {
		return
	}

	user = slackUserFromDto(userInfo)
	sc.userInfo[userInfo.ID] = user
	return
}

// ResolveChannel takes a slackID and fetches a SlackChannel for the ID
func (sc *SlackClient) ResolveChannel(slackID string) (channel *SlackChannel, err error) {
	channel, found := sc.channelInfo[slackID]
	if found {
		return
	}

	channelInfo, err := sc.client.GetConversationInfo(slackID, false)
	if err != nil {
		return
	}

	channel = slackChannelFromDto(channelInfo)
	sc.channelInfo[channelInfo.ID] = channel
	return
}

// Message represents text uttered in a Slack channel
type Message struct {
	Nick    string
	Channel string
	Data    string
}

// ClientChans contains a sending channel, receiving channel, and stop channel
// that the Slack goroutine receives from, sends to, and can stop according to
type ClientChans struct {
	RecvChan <-chan string
	SendChan chan<- *Message
	StopChan <-chan bool
}

// Initialize bootstraps the SlackClient with a client token and loads data
func (sc *SlackClient) Initialize(token string) {
	sc.client = slack.New(token)
	sc.rtm = sc.client.NewRTM()
	sc.bootstrapMappings()
}

// Poop is a goroutine entry point that handles the communication with Slack
func (sc *SlackClient) Poop(chans *ClientChans) {
	go sc.rtm.ManageConnection()
	defer sc.rtm.Disconnect()

	for {
		select {
		case <-chans.StopChan:
			return

		default:
			event := <-sc.rtm.IncomingEvents
			switch event.Type {
			case "message":
				messageData, ok := event.Data.(*slack.MessageEvent)
				if !ok {
					chans.SendChan <- &Message{
						"*tanya", "*tanya",
						fmt.Sprintf("Non message-event: %v", event.Data),
					}
					break
				}

				switch messageData.SubType {
				case "":
					user, err := sc.ResolveUser(messageData.User)
					if err != nil {
						log.Println(err)
						continue
					}
					channel, err := sc.ResolveChannel(messageData.Channel)
					if err != nil {
						log.Println(err)
						continue
					}

					if messageData.Text != "" {
						chans.SendChan <- &Message{
							user.Nick, channel.Name, sc.ParseMessageText(messageData.Text),
						}

						if messageData.Text == "hallo!" {
							sc.rtm.SendMessage(&slack.OutgoingMessage{
								ID:      1,
								Channel: messageData.Channel,
								Text:    "hullo!",
								Type:    "message",
							})
						}

					} else {
						// Maybe we have an attachment instead.
						for _, attachment := range messageData.Attachments {
							chans.SendChan <- &Message{
								user.Nick,
								channel.Name,
								sc.slackUrlDecoder.Replace(attachment.Fallback),
							}
						}
					}

				default:
					chans.SendChan <- &Message{
						"*tanya", "*tanya",
						fmt.Sprintf("%v: %+v", event.Type, event.Data),
					}
				}

			case "channel_marked", "latency_report", "user_typing", "pref_change":
				// haha nobody cares about this

			default:
				chans.SendChan <- &Message{
					"*tanya", "*tanya",
					fmt.Sprintf("%v: %+v", event.Type, event.Data),
				}
			}
		}
	}
}
