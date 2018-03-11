package gateway

import (
	"fmt"
	"log"

	"github.com/nlopes/slack"
)

type SlackChannel struct {
	SlackId string
	Name    string

	Topic slack.Topic
}

func slackChannelFromDto(channel *slack.Channel) *SlackChannel {
	return &SlackChannel{
		SlackId: channel.ID,
		Name:    channel.Name,
		Topic:   channel.Topic,
	}
}

type SlackUser struct {
	SlackId     string
	DisplayName string
	RealName    string
}

func slackUserFromDto(user *slack.User) *SlackUser {
	return &SlackUser{
		SlackId:     user.ID,
		DisplayName: user.Name,
		RealName:    user.RealName,
	}
}

type SlackClient struct {
	client *slack.Client
	rtm    *slack.RTM

	channelInfo map[string]*SlackChannel
	userInfo    map[string]*SlackUser
}

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

func (sc *SlackClient) ResolveUser(slackId string) (user *SlackUser, err error) {
	user, found := sc.userInfo[slackId]
	if found {
		return
	}

	userInfo, err := sc.client.GetUserInfo(slackId)
	if err != nil {
		return
	}

	user = slackUserFromDto(userInfo)
	sc.userInfo[userInfo.ID] = user
	return
}

func (sc *SlackClient) ResolveChannel(slackId string) (channel *SlackChannel, err error) {
	channel, found := sc.channelInfo[slackId]
	if found {
		return
	}

	channelInfo, err := sc.client.GetConversationInfo(slackId, false)
	if err != nil {
		return
	}

	channel = slackChannelFromDto(channelInfo)
	sc.channelInfo[channelInfo.ID] = channel
	return
}

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
				}
				channel, err := slackClient.ResolveChannel(messageData.Channel)
				if err != nil {
					log.Println(err)
				}

				if user == nil || channel == nil {

					continue
				}

				fmt.Printf(":%v!%[1]v@localhost PRIVMSG #%v :%v\n", user.DisplayName, channel.Name, messageData.Text)
				if messageData.Text == "hallo!" {
					rtm.SendMessage(&slack.OutgoingMessage{
						ID:      1,
						Channel: messageData.Channel,
						Text:    "hullo!",
						Type:    "message",
					})
				}

			default:
				fmt.Printf("%v: %+v\n", event.Type, event.Data)
			}

		case "channel_marked", "latency_report", "user_typing":
			// haha nobody cares about this

		default:
			fmt.Printf("%v: %+v\n", event.Type, event.Data)
		}
	}
}
