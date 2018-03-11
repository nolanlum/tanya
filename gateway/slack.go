package gateway

import (
	"fmt"
	"log"

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
		Name:    channel.Name,
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
}

// BootstrapMappings makes the initial Slack calls to bootstrap
// our connection
func (sc *SlackClient) BootstrapMappings() {
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

// Poop handles the communication with Slack
func Poop(token string) {
	client := slack.New(token)
	rtm := client.NewRTM()
	go rtm.ManageConnection()

	slackClient := SlackClient{
		client:      client,
		rtm:         rtm,
		channelInfo: make(map[string]*SlackChannel),
		userInfo:    make(map[string]*SlackUser),
	}
	slackClient.BootstrapMappings()

	for {
		event := <-rtm.IncomingEvents
		switch event.Type {
		case "message":
			messageData := event.Data.(*slack.MessageEvent)
			switch messageData.SubType {
			case "":
				user, err := slackClient.ResolveUser(messageData.User)
				if err != nil {
					log.Println(err)
					continue
				}
				channel, err := slackClient.ResolveChannel(messageData.Channel)
				if err != nil {
					log.Println(err)
					continue
				}

				if messageData.Text != "" {
					fmt.Printf(":%v!%[1]v@localhost PRIVMSG #%v :%v\n",
						user.Nick, channel.Name, slackClient.ParseMessageText(messageData.Text))

					if messageData.Text == "hallo!" {
						rtm.SendMessage(&slack.OutgoingMessage{
							ID:      1,
							Channel: messageData.Channel,
							Text:    "hullo!",
							Type:    "message",
						})
					}

				} else {
					// Maybe we have an attachment instead.
					for _, attachment := range messageData.Attachments {
						fmt.Printf(":%v!%[1]v@localhost PRIVMSG #%v :%v\n",
							user.Nick, channel.Name, attachment.Fallback)
					}
				}

			default:
				fmt.Printf("%v: %+v\n", event.Type, event.Data)
			}

		case "channel_marked", "latency_report", "user_typing", "pref_change":
			// haha nobody cares about this

		default:
			fmt.Printf("%v: %+v\n", event.Type, event.Data)
		}
	}
}
