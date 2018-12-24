package gateway

import (
	"fmt"
	"log"
	"os"
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
	dmInfo             map[string]*SlackUser
	channelMemberships map[string]*SlackChannel
	channelMembers     map[string]map[string]*SlackUser

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
		dmInfo:             make(map[string]*SlackUser),
		channelMemberships: make(map[string]*SlackChannel),
		channelMembers:     make(map[string]map[string]*SlackUser),

		nickToUserMap:      make(map[string]string),
		channelNameToIDMap: make(map[string]string),

		slackURLEncoder: strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;"),
		slackURLDecoder: strings.NewReplacer("&gt;", ">", "&lt;", "<", "&amp;", "&"),
	}
}

// Clear all stored state and reload workspace/conversation metadata from Slack.
// Called upon reconnection to ensure all cached state is up-to-date.
func (sc *SlackClient) bootstrapMappings() {
	startTime := time.Now()

	channelInfo := make(map[string]*SlackChannel)
	userInfo := make(map[string]*SlackUser)
	dmInfo := make(map[string]*SlackUser)
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

	ims, err := sc.client.GetIMChannels()
	if err != nil {
		log.Fatalln(err)
	}
	for _, im := range ims {
		dmInfo[im.ID] = userInfo[im.User]
	}

	sc.Lock()
	sc.channelInfo = channelInfo
	sc.userInfo = userInfo
	sc.dmInfo = dmInfo
	sc.channelMemberships = channelMemberships
	sc.channelMembers = make(map[string]map[string]*SlackUser)
	sc.Unlock()

	sc.regenerateReverseMappings()

	log.Printf("slack:init channels:%v users:%v dms:%v memberships:%v time:%v",
		len(sc.channelInfo), len(sc.userInfo), len(sc.dmInfo), len(sc.channelMemberships), time.Since(startTime))
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
	sc.RLock()
	user, found := sc.userInfo[slackID]
	if found {
		sc.RUnlock()
		return
	}

	sc.RUnlock()
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
	sc.RLock()
	channel, found := sc.channelInfo[slackID]
	if found {
		sc.RUnlock()
		return
	}

	sc.RUnlock()
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

// ResolveDMToUser resolves a DM/IM Channel ID to the User the DM is for
func (sc *SlackClient) ResolveDMToUser(dmChannelID string) (*SlackUser, error) {
	sc.RLock()
	slackUser, found := sc.dmInfo[dmChannelID]
	sc.RUnlock()

	if found {
		return slackUser, nil
	}

	slackUser = nil
	ims, err := sc.client.GetIMChannels()
	if err != nil {
		return nil, err
	}

	sc.Lock()
	for _, im := range ims {
		// Skip this IM if we cannot find the user it belongs to
		if userInfo, found := sc.userInfo[im.User]; found {
			sc.dmInfo[im.ID] = userInfo
			if im.ID == dmChannelID {
				slackUser = userInfo
			}
		}
	}
	sc.Unlock()

	if slackUser != nil {
		return slackUser, nil
	}

	return nil, fmt.Errorf("could not find user for DM: %s", dmChannelID)
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
func (sc *SlackClient) Initialize(token string, debug bool) {
	sc.client = slack.New(token)
	sc.rtm = sc.client.NewRTM()

	if debug {
		slack.SetLogger(log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile))
		sc.client.SetDebug(true)
		sc.rtm.SetDebug(true)
	}
}

// SendMessage sends a message to a SlackChannel
func (sc *SlackClient) SendMessage(channel *SlackChannel, msg string) error {
	msg = sc.UnparseMessageText(msg)
	sc.rtm.SendMessage(sc.rtm.NewOutgoingMessage(msg, channel.SlackID))
	return nil
}

// SendDirectMessage sends a message to a SlackUser
func (sc *SlackClient) SendDirectMessage(user *SlackUser, msg string) error {
	msg = sc.UnparseMessageText(msg)
	_, _, imChannelID, err := sc.client.OpenIMChannel(user.SlackID)
	if err != nil {
		return err
	}
	sc.rtm.SendMessage(sc.rtm.NewOutgoingMessage(msg, imChannelID))
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

func isDmChannel(channelID string) bool {
	return len(channelID) > 0 && channelID[0] == 'D'
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

			case "incoming_error":
				incomingEventError := event.Data.(*slack.IncomingEventError)
				chans.IncomingChan <- sc.newInternalMessageEvent(fmt.Sprintf(
					"incoming error from slack, disconnecting: %v", incomingEventError.Error()))

			case "connecting":
				connectingData := event.Data.(*slack.ConnectingEvent)

				verb := "connecting"
				if connectingData.ConnectionCount > 1 {
					verb = "reconnecting"
				}
				chans.IncomingChan <- sc.newInternalMessageEvent(fmt.Sprintf(
					"%s to slack (attempt %d)", verb, connectingData.Attempt))

			case "connected":
				connectedData := event.Data.(*slack.ConnectedEvent)
				sc.bootstrapMappings()
				go sc.bootstrapChannelUserList()
				sc.self = sc.userInfo[connectedData.Info.User.ID]

				log.Printf("tanya connected to slack as %v\n", sc.self)

				chans.IncomingChan <- &SlackEvent{
					EventType: SlackConnectedEvent,
					Data: &SlackConnectedEventData{
						UserInfo: sc.self,
					},
				}

			case "hello":
				chans.IncomingChan <- sc.newInternalMessageEvent("connected to slack!")

			case "disconnected":
				log.Println("tanya disconnected from slack")
				chans.IncomingChan <- sc.newInternalMessageEvent("disconnected from slack!")

			case "message":
				messageData := event.Data.(*slack.MessageEvent)
				sc.handleMessageEvent(chans.IncomingChan, messageData)

			case "user_change":
				userData := event.Data.(*slack.UserChangeEvent)

				// Update user info based on the new DTO
				oldUserInfo := sc.userInfo[userData.User.ID]
				newUserInfo := slackUserFromDto(&userData.User)

				// Atomically replace the old user info object with the new
				// Here we also need to update sc.self if our user info was updated
				sc.Lock()
				sc.userInfo[newUserInfo.SlackID] = newUserInfo
				delete(sc.nickToUserMap, oldUserInfo.Nick)
				sc.nickToUserMap[newUserInfo.Nick] = newUserInfo.SlackID
				if userData.User.ID == sc.self.SlackID {
					sc.self = newUserInfo
				}
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

			case "file_shared":
				fileSharedEvent := event.Data.(*slack.FileSharedEvent)
				file := fileSharedEvent.File
				if len(file.Channels) == 0 {
					continue
				}

				target, err := sc.ResolveChannel(file.Channels[0])
				if err != nil {
					log.Println(err)
					continue
				}

				user, err := sc.ResolveUser(file.User)
				if err != nil {
					log.Println(err)
					continue
				}

				shareMessage := fmt.Sprintf(
					"@%s shared a file: %s %s", user.Nick, file.Name, file.URLPrivateDownload,
				)
				chans.IncomingChan <- newSlackMessageEvent(user, target.Name, sc.slackURLDecoder.Replace(shareMessage))

			case "member_joined_channel":
				memberJoinedChannelEvent := event.Data.(*slack.MemberJoinedChannelEvent)
				joinEvent, err := sc.handleMemberJoinedChannel(
					memberJoinedChannelEvent.Channel, memberJoinedChannelEvent.User)

				if err != nil {
					joinEvent = sc.newInternalMessageEvent(fmt.Sprintf(
						"error handling member_joined_channel event [%v]: %+v", err, memberJoinedChannelEvent))
				}

				chans.IncomingChan <- joinEvent

			case "member_left_channel":
				memberLeftChannelEvent := event.Data.(*slack.MemberLeftChannelEvent)
				partEvent, err := sc.handleMemberLeftChannel(
					memberLeftChannelEvent.Channel, memberLeftChannelEvent.User)

				if err != nil {
					partEvent = sc.newInternalMessageEvent(fmt.Sprintf(
						"error handling member_left_channel event [%v]: %+v", err, memberLeftChannelEvent))
				}

				chans.IncomingChan <- partEvent

			case "channel_marked", "group_marked", "thread_marked", "im_marked",
				"latency_report", "user_typing", "pref_change", "dnd_updated_user",
				"file_created", "file_public",
				"reaction_added", "reaction_removed", "pin_added", "pin_removed":
				// haha nobody cares about this

			case "ack":
				// maybe we care about this
				if ack, ok := event.Data.(*slack.AckMessage); ok && ack.Ok {
					continue
				}
				fallthrough

			default:
				log.Printf("unhandled event [%v]: %+v", event.Type, event.Data)
			}
		}
	}
}
