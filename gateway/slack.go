package gateway

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/slack-go/slack"
)

// SlackChannel holds data for a channel on Slack
type SlackChannel struct {
	SlackID string
	Name    string
	Created time.Time
	Private bool

	Topic slack.Topic
}

func slackChannelFromDto(channel *slack.Channel) *SlackChannel {
	return &SlackChannel{
		SlackID: channel.ID,
		Name:    "#" + channel.Name,
		Created: channel.Created.Time(),
		Private: channel.IsGroup,
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

type inflightMessage struct {
	MessageText        string
	DestinationSlackID string
	InitialMessageID   int

	RetriesRemaining int
	RetryInterval    time.Duration
}

func newInflightMessage(msg string, destinationSlackID string, initialMessageID int) *inflightMessage {
	return &inflightMessage{
		MessageText:        msg,
		DestinationSlackID: destinationSlackID,
		InitialMessageID:   initialMessageID,

		RetriesRemaining: 3,
		RetryInterval:    time.Second + (time.Duration(rand.Intn(1000)) * time.Millisecond),
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
	userIDToDMIDMap    map[string]string

	inflightMessageMap map[int]*inflightMessage
	slackURLEncoder    *strings.Replacer
	slackURLDecoder    *strings.Replacer
	conversationMarker *ConversationMarker

	threadTimestamps    *lru.Cache
	threadQuoteInterval int

	sync.RWMutex
}

// NewSlackClient creates a new SlackClient with some default values
func NewSlackClient(threadQuoteInterval int) *SlackClient {
	sc := &SlackClient{
		channelInfo:        make(map[string]*SlackChannel),
		userInfo:           make(map[string]*SlackUser),
		dmInfo:             make(map[string]*SlackUser),
		channelMemberships: make(map[string]*SlackChannel),
		channelMembers:     make(map[string]map[string]*SlackUser),

		nickToUserMap:      make(map[string]string),
		channelNameToIDMap: make(map[string]string),
		userIDToDMIDMap:    make(map[string]string),

		slackURLEncoder:    strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;"),
		slackURLDecoder:    strings.NewReplacer("&gt;", ">", "&lt;", "<", "&amp;", "&"),
		conversationMarker: NewConversationMarker(),

		threadQuoteInterval: threadQuoteInterval,
	}

	var err error
	if sc.threadTimestamps, err = lru.New(128); err != nil {
		log.Panicf("Could not create new LRU: %v", err)
	}

	return sc
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
	if debug {
		sc.client = slack.New(
			token, slack.OptionDebug(true), slack.OptionLog(log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)))
	} else {
		sc.client = slack.New(token)
	}

	sc.rtm = sc.client.NewRTM()
}

// SendMessage sends a message to a SlackChannel
func (sc *SlackClient) SendMessage(channel *SlackChannel, msg string) error {
	msg = sc.UnparseMessageText(msg)
	outgoingMessage := sc.rtm.NewOutgoingMessage(msg, channel.SlackID)

	if channel.Private {
		sc.conversationMarker.MarkGroup(sc.client, channel.SlackID, outgoingMessage.ID)
	} else {
		sc.conversationMarker.MarkChannel(sc.client, channel.SlackID, outgoingMessage.ID)
	}

	sc.inflightMessageMap[outgoingMessage.ID] = newInflightMessage(msg, channel.SlackID, outgoingMessage.ID)
	sc.rtm.SendMessage(outgoingMessage)
	return nil
}

// SendDirectMessage sends a message to a SlackUser
func (sc *SlackClient) SendDirectMessage(user *SlackUser, msg string) error {
	imChannelID, err := sc.ResolveUserToDM(user)
	if err != nil {
		return err
	}

	msg = sc.UnparseMessageText(msg)
	outgoingMessage := sc.rtm.NewOutgoingMessage(msg, imChannelID)
	sc.conversationMarker.MarkDM(sc.client, imChannelID, outgoingMessage.ID)
	sc.inflightMessageMap[outgoingMessage.ID] = newInflightMessage(msg, imChannelID, outgoingMessage.ID)
	sc.rtm.SendMessage(outgoingMessage)
	return nil
}

func (sc *SlackClient) retryInflightMessage(msg *inflightMessage) {
	// Process retry backoff.
	time.Sleep(msg.RetryInterval)
	msg.RetriesRemaining -= 1
	msg.RetryInterval = 2*msg.RetryInterval.Truncate(time.Second) + time.Duration(rand.Intn(1000))*time.Millisecond

	// Create new RTM outgoing message and add to tracking map.
	outgoingMessage := sc.rtm.NewOutgoingMessage(msg.MessageText, msg.DestinationSlackID)
	sc.inflightMessageMap[outgoingMessage.ID] = msg

	// Send.
	sc.rtm.SendMessage(outgoingMessage)
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
				log.Printf("%s slack connection error: %v\n", sc.Tag(), connEventError.Error())

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

				log.Printf("%s[#%d] tanya connecting to slack (attempt %d)",
					sc.Tag(), connectingData.ConnectionCount, connectingData.Attempt)
				chans.IncomingChan <- sc.newInternalMessageEvent(fmt.Sprintf(
					"%s to slack (attempt %d)", verb, connectingData.Attempt))

			case "connected":
				connectedData := event.Data.(*slack.ConnectedEvent)
				sc.bootstrapMappings()
				go sc.bootstrapChannelUserList()
				sc.self = sc.userInfo[connectedData.Info.User.ID]

				log.Printf("%s tanya connected to slack as %v\n", sc.Tag(), sc.self)

				chans.IncomingChan <- &SlackEvent{
					EventType: SlackConnectedEvent,
					Data: &SlackConnectedEventData{
						UserInfo: sc.self,
					},
				}

			case "hello":
				chans.IncomingChan <- sc.newInternalMessageEvent("connected to slack!")

			case "disconnected":
				disconnectedData := event.Data.(*slack.DisconnectedEvent)
				log.Printf("%s disconnected from slack: %v", sc.Tag(), disconnectedData.Cause)
				chans.IncomingChan <- sc.newInternalMessageEvent("disconnected from slack!")

			case "message":
				messageData := event.Data.(*slack.MessageEvent)
				sc.handleMessageEvent(chans.IncomingChan, messageData)

			case "user_change":
				userData := event.Data.(*slack.UserChangeEvent)
				newUserInfo := slackUserFromDto(&userData.User)

				// Atomically check and replace the old user info object with the new
				sc.Lock()
				oldUserInfo, hadOldUserInfo := sc.userInfo[userData.User.ID]
				sc.userInfo[newUserInfo.SlackID] = newUserInfo

				// Un-map the old nick, if we had one, and insert an entry for the new
				if hadOldUserInfo {
					delete(sc.nickToUserMap, oldUserInfo.Nick)
				}
				sc.nickToUserMap[newUserInfo.Nick] = newUserInfo.SlackID

				// Here we also need to update sc.self if our user info was updated
				if userData.User.ID == sc.self.SlackID {
					sc.self = newUserInfo
				}
				sc.Unlock()

				// Send nick change event if necessary
				if hadOldUserInfo && (oldUserInfo.Nick != newUserInfo.Nick) {
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
					log.Printf("%s %v", sc.Tag(), err)
					continue
				}

				user, err := sc.ResolveUser(file.User)
				if err != nil {
					log.Printf("%s %v", sc.Tag(), err)
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

			case "channel_marked", "group_marked", "thread_marked", "im_marked", "im_open",
				"latency_report", "user_typing", "pref_change", "dnd_updated_user", "desktop_notification",
				"file_created", "file_public",
				"reaction_added", "reaction_removed", "pin_added", "pin_removed":
				// haha nobody cares about this

			case "ack_error":
				ackError := event.Data.(*slack.AckErrorEvent)
				if inflightMessage, found := sc.inflightMessageMap[ackError.ReplyTo]; found {
					delete(sc.inflightMessageMap, ackError.ReplyTo)
					chans.IncomingChan <- sc.newInternalMessageEvent(fmt.Sprintf(
						"failed to send message [%v], retry in %ds: %v",
						inflightMessage.MessageText,
						inflightMessage.RetryInterval.Truncate(time.Second)/time.Second,
						ackError.Error(),
					))
					go sc.retryInflightMessage(inflightMessage)
				}

			case "ack":
				// maybe we care about this
				if ack, ok := event.Data.(*slack.AckMessage); ok && ack.Ok {
					if inflightMessage, found := sc.inflightMessageMap[ack.ReplyTo]; found {
						delete(sc.inflightMessageMap, ack.ReplyTo)
						sc.conversationMarker.HandleRTMAck(inflightMessage.InitialMessageID, ack.Timestamp)
					} else {
						sc.conversationMarker.HandleRTMAck(ack.ReplyTo, ack.Timestamp)
					}
					continue
				}
				fallthrough

			default:
				log.Printf("%s unhandled event [%v]: %+v", sc.Tag(), event.Type, event.Data)
			}
		}
	}
}

// Tag is a descriptor of the SlackClient suitable for logging or simple human identification.
func (sc *SlackClient) Tag() string {
	switch sc.self {
	case nil:
		return fmt.Sprintf("[%-12p]", sc)
	default:
		return fmt.Sprintf("[%-12s]", sc.self.SlackID)
	}
}
