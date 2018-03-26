package gateway

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/nlopes/slack"
)

// SlackChannel holds data for a channel on Slack
type SlackChannel struct {
	SlackID string
	Name    string
	Created time.Time

	Topic slack.Topic
}

func slackChannelFromDto(channel *slack.Channel) *SlackChannel {
	return &SlackChannel{
		SlackID: channel.ID,
		Name:    "#" + channel.Name,
		Created: channel.Created.Time(),
		Topic:   channel.Topic,
	}
}

// SlackUser holds data for each user on Slack
type SlackUser struct {
	SlackID  string
	Nick     string
	RealName string
}

var tanyaInternalUser = &SlackUser{SlackID: "tanya", Nick: "*tanya"}

func slackUserFromDto(user *slack.User) *SlackUser {
	// !sux slack
	nick := user.Profile.DisplayNameNormalized
	if nick == "" {
		nick = user.Profile.RealNameNormalized
	}
	nick = strings.Replace(nick, " ", "\u00a0", -1)

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
	self   *SlackUser

	channelInfo        map[string]*SlackChannel
	userInfo           map[string]*SlackUser
	channelMemberships map[string]*SlackChannel

	nickToUserMap      map[string]string
	channelNameToIDMap map[string]string

	slackURLEncoder *strings.Replacer
	slackURLDecoder *strings.Replacer

	sync.RWMutex
}

// NewSlackClient creates a new SlackClient with some default values
func NewSlackClient() *SlackClient {
	return &SlackClient{
		channelInfo:        make(map[string]*SlackChannel),
		userInfo:           make(map[string]*SlackUser),
		channelMemberships: make(map[string]*SlackChannel),

		slackURLEncoder: strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;"),
		slackURLDecoder: strings.NewReplacer("&gt;", ">", "&lt;", "<", "&amp;", "&"),
	}
}

// Make the initial Slack calls to bootstrap our connection
func (sc *SlackClient) bootstrapMappings() {
	channelInfo := make(map[string]*SlackChannel)
	userInfo := make(map[string]*SlackUser)
	channelMemberships := make(map[string]*SlackChannel)

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
			log.Fatalln(err)
		}

		for _, channel := range channels {
			slackChannel := slackChannelFromDto(&channel)

			channelInfo[channel.ID] = slackChannel
			if channel.IsMember {
				channelMemberships[channel.ID] = slackChannel
			}
		}

		hasMore = gcp.Cursor != ""
	}

	users, err := sc.client.GetUsers()
	if err != nil {
		log.Fatalln(err)
	}
	for _, user := range users {
		userInfo[user.ID] = slackUserFromDto(&user)
	}

	sc.Lock()
	sc.channelInfo = channelInfo
	sc.userInfo = userInfo
	sc.channelMemberships = channelMemberships
	sc.Unlock()

	sc.regenerateReverseMappings()
}

// Regenerate the cached reverse nick/channel name mappings
// If two channels have the same name, then whelp the first one we find wins
func (sc *SlackClient) regenerateReverseMappings() {
	sc.Lock()
	defer sc.Unlock()

	sc.nickToUserMap = make(map[string]string)
	for _, user := range sc.userInfo {
		sc.nickToUserMap[user.Nick] = user.SlackID
	}

	sc.channelNameToIDMap = make(map[string]string)
	for _, channel := range sc.channelInfo {
		sc.channelNameToIDMap[channel.Name] = channel.SlackID
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

	sc.Lock()
	sc.userInfo[user.SlackID] = user
	sc.nickToUserMap[user.Nick] = user.SlackID
	sc.Unlock()
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

	sc.Lock()
	sc.channelInfo[channel.SlackID] = channel
	sc.channelNameToIDMap[channel.Name] = channel.SlackID
	sc.Unlock()
	return
}

// ResolveNameToChannel takes a channel name and fetches a SlackChannel with that name
func (sc *SlackClient) ResolveNameToChannel(channelName string) *SlackChannel {
	sc.RLock()
	defer sc.RUnlock()

	if channelID, found := sc.channelNameToIDMap[channelName]; found {
		if channelInfo, found := sc.channelInfo[channelID]; found {
			if channelInfo.Name != channelName {
				log.Panicf("SlackClient.channelNameToIDMap had stale data: %v = %v != %v",
					channelName, channelID, channelInfo.Name)
			}

			return channelInfo
		}
	}

	return nil
}

// ResolveNickToUser takes a nick and fetches a SlackUser with that nick
func (sc *SlackClient) ResolveNickToUser(nick string) *SlackUser {
	sc.RLock()
	defer sc.RUnlock()

	if userID, found := sc.nickToUserMap[nick]; found {
		if userInfo, found := sc.userInfo[userID]; found {
			if userInfo.Nick != nick {
				log.Panicf("SlackClient.nickToUserMap had stale data: %v = %v != %v", nick, userID, userInfo.Nick)
			}

			return userInfo
		}
	}

	return nil
}

// GetChannelUsers queries the Slack API for a list of users in the given channel, returning
// SlackUser objects for each one
func (sc *SlackClient) GetChannelUsers(channelID string) (users []SlackUser, err error) {
	hasMore := true
	guicp := &slack.GetUsersInConversationParameters{
		ChannelID: channelID,
		Limit:     1000,
	}
	for hasMore {
		var userIDs []string
		userIDs, guicp.Cursor, err = sc.client.GetUsersInConversation(guicp)
		if err != nil {
			return
		}

		for _, userID := range userIDs {
			var user *SlackUser
			user, err = sc.ResolveUser(userID)
			if err != nil {
				return
			}
			users = append(users, *user)
		}

		hasMore = guicp.Cursor != ""
	}

	return
}

// GetChannelMemberships returns the channels this SlackClient is a member of
func (sc *SlackClient) GetChannelMemberships() (channels []SlackChannel) {
	sc.RLock()
	defer sc.RUnlock()

	for _, channel := range sc.channelMemberships {
		channels = append(channels, *channel)
	}
	return
}

// ClientChans contains a sending channel, receiving channel, and stop channel
// that the Slack goroutine receives outgoing commands from, sends incoming messages to,
// and can stop according to
type ClientChans struct {
	OutgoingChan <-chan string
	IncomingChan chan<- *SlackEvent
	StopChan     <-chan interface{}
}

// Initialize bootstraps the SlackClient with a client token
func (sc *SlackClient) Initialize(token string) {
	sc.client = slack.New(token)
	sc.rtm = sc.client.NewRTM()
}

// SendMessage sends a message to a SlackChannel
func (sc *SlackClient) SendMessage(channel *SlackChannel, msg string) error {
	msg = sc.UnparseMessageText(msg)
	sc.rtm.SendMessage(sc.rtm.NewOutgoingMessage(msg, channel.SlackID))
	return nil
}

func newSlackMessageEvent(from *SlackUser, target, message string) *SlackEvent {
	return &SlackEvent{
		EventType: MessageEvent,
		Data:      &MessageEventData{*from, target, message},
	}
}

func (sc *SlackClient) newInternalMessageEvent(message string) *SlackEvent {
	to := tanyaInternalUser.Nick
	if sc.self != nil {
		to = sc.self.Nick
	}

	return newSlackMessageEvent(tanyaInternalUser, to, message)
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
			case "connection_error":
				connEventError := event.Data.(*slack.ConnectionErrorEvent)
				log.Printf("error connecting to slack: %v\n", connEventError.Error())

			case "connected":
				connectedData := event.Data.(*slack.ConnectedEvent)
				sc.bootstrapMappings()
				sc.self = sc.userInfo[connectedData.Info.User.ID]

				log.Printf("tanya connected to slack as %v\n", sc.self)

				chans.IncomingChan <- &SlackEvent{
					EventType: SlackConnectedEvent,
					Data: &SlackConnectedEventData{
						UserInfo: sc.self,
					},
				}

			case "message":
				messageData := event.Data.(*slack.MessageEvent)

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
						chans.IncomingChan <- newSlackMessageEvent(
							user, channel.Name, sc.ParseMessageText(messageData.Text))
					} else {
						// Maybe we have an attachment instead.
						for _, attachment := range messageData.Attachments {
							chans.IncomingChan <- newSlackMessageEvent(
								user, channel.Name, sc.slackURLDecoder.Replace(attachment.Fallback))
						}
					}

				case "message_changed":
					if messageData.SubMessage == nil || messageData.SubMessage.SubType != "" {
						chans.IncomingChan <- sc.newInternalMessageEvent(fmt.Sprintf("%+v", messageData))
						continue
					}
					subMessage := messageData.SubMessage

					// For now, only handle the Slack native expansion of archive links
					if !strings.Contains(subMessage.Text, "slack.com/archives") || len(subMessage.Attachments) < 1 {
						continue
					}

					user, err := sc.ResolveUser(subMessage.User)
					if err != nil {
						log.Println(err)
						continue
					}
					channel, err := sc.ResolveChannel(messageData.Channel)
					if err != nil {
						log.Println(err)
						continue
					}
					quotedUser, err := sc.ResolveUser(subMessage.Attachments[0].AuthorId)
					if err != nil {
						log.Println(err)
						continue
					}
					chans.IncomingChan <- newSlackMessageEvent(
						user,
						channel.Name,
						fmt.Sprintf(
							"<%s> %s",
							quotedUser.Nick,
							sc.ParseMessageText(subMessage.Attachments[0].Text),
						),
					)

				case "channel_topic":
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

					chans.IncomingChan <- &SlackEvent{
						EventType: TopicChangeEvent,
						Data: &TopicChangeEventData{
							From:     *user,
							Target:   channel.Name,
							NewTopic: messageData.Topic,
						},
					}

				default:
					chans.IncomingChan <- sc.newInternalMessageEvent(fmt.Sprintf("%v: %+v", event.Type, event.Data))
				}

			case "user_change":
				userData := event.Data.(*slack.UserChangeEvent)

				// Update user info based on the new DTO
				oldUserInfo := sc.userInfo[userData.User.ID]
				newUserInfo := slackUserFromDto(&userData.User)

				sc.Lock()
				sc.userInfo[newUserInfo.SlackID] = newUserInfo
				delete(sc.nickToUserMap, oldUserInfo.Nick)
				sc.nickToUserMap[newUserInfo.Nick] = newUserInfo.SlackID
				sc.Unlock()

				// Send nick change event if necessary
				if oldUserInfo.Nick != newUserInfo.Nick {
					chans.IncomingChan <- &SlackEvent{
						EventType: NickChangeEvent,
						Data: &NickChangeEventData{
							From:    *oldUserInfo,
							NewNick: newUserInfo.Nick,
						},
					}
				}

			case "channel_marked", "group_marked", "latency_report", "user_typing", "pref_change":
				// haha nobody cares about this

			case "ack":
				// maybe we care about this
				if ack, ok := event.Data.(*slack.AckMessage); ok && ack.Ok {
					continue
				}
				fallthrough

			default:
				chans.IncomingChan <- sc.newInternalMessageEvent(fmt.Sprintf("%v: %+v", event.Type, event.Data))
			}
		}
	}
}
