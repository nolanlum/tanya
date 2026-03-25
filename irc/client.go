package irc

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

type clientState int

const (
	clientStateRegistering clientState = iota
	clientStateAwaitingNick
	clientStateAwaitingUser
	clientStateRegistered
)

type clientConnection struct {
	conn          *net.TCPConn
	config        *Config
	stateProvider ServerStateProvider

	clientUser User
	serverUser *User

	state       clientState
	joinedChans map[string]struct{}

	outgoingMessages chan *Message
	serverChan       chan<- *ServerMessage

	slackConnected <-chan struct{}
	shutdown       chan struct{}
}

func newClientConnection(
	conn *net.TCPConn,
	user *User,
	config *Config,
	stateProvider ServerStateProvider,
	serverChan chan *ServerMessage,
	slackConnectedChan <-chan struct{},
) *clientConnection {
	ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

	return &clientConnection{
		conn:          conn,
		config:        config,
		stateProvider: stateProvider,

		clientUser: User{Nick: "*", Ident: user.Ident, Host: ip},
		serverUser: user,

		joinedChans: make(map[string]struct{}),

		outgoingMessages: make(chan *Message),
		serverChan:       serverChan,

		slackConnected: slackConnectedChan,
		shutdown:       make(chan struct{}),
	}
}

func (cc *clientConnection) String() string {
	return fmt.Sprintf("%v", cc.conn.RemoteAddr())
}

func (cc *clientConnection) finishRegistration() {
	// Set the client straight if its nick is wrong
	if cc.clientUser.Nick != cc.serverUser.Nick {
		cc.outgoingMessages <- (&Nick{
			From:    User{Nick: cc.clientUser.Nick, Ident: cc.serverUser.Ident},
			NewNick: cc.serverUser.Nick,
		}).ToMessage()
	}
	cc.clientUser = *cc.serverUser
	cc.sendWelcome()
}

func (cc *clientConnection) handleConnInput() {
	defer cc.conn.Close()

	// Wait for Slack connect burst before handling incoming messages.
	select {
	case <-time.After(2 * time.Second):
		// Write a raw IRC line here, since the outgoing message channel eats any messages
		// sent before client registration.
		fmt.Fprintf(cc.conn, ":%s NOTICE * :Waiting for Slack connection...\n", cc.config.ServerName)
		<-cc.slackConnected
	case <-cc.slackConnected:
	}

	s := bufio.NewScanner(cc.conn)

SelectLoop:
	for {
		select {
		case <-cc.shutdown:
			return

		default:
			if !s.Scan() {
				close(cc.shutdown)
				continue
			}
			msgStr := s.Text()
			msg, err := StringToMessage(msgStr)

			// Pretty sure error handling shouldn't be this complicated
			if err != nil {
				if err == ErrMalformedIRCMessage {
					log.Printf("[%v] sent malformed IRC message: %v", cc, msgStr)
				} else if numeric, ok := err.(*NumericReply); ok {
					fmt.Fprintln(cc.conn, cc.reply(*numeric).String())
				} else {
					log.Printf("[%v] error: %v", cc, err)
				}
				continue
			}

			switch msg.Cmd {
			case PrivmsgCmd:
				// Swallow the PRIVMSG if we haven't registered yet
				if cc.state != clientStateRegistered {
					continue
				}

				messagable, err := ParseMessage(msg)
				if err == nil {
					cc.serverChan <- &ServerMessage{
						message: messagable,
						cAddr:   cc.conn.RemoteAddr(),
					}

				}
			case NickCmd:
				if cc.state == clientStateRegistering {
					cc.clientUser.Nick = msg.Params[0]
					cc.state = clientStateAwaitingUser
				} else if cc.state == clientStateAwaitingNick {
					// Finish registration if we already have the USER
					cc.clientUser.Nick = msg.Params[0]
					cc.state = clientStateRegistered
					cc.finishRegistration()
				} else {
					cc.outgoingMessages <- (&Nick{
						From:    User{Nick: msg.Params[0], Ident: cc.serverUser.Ident},
						NewNick: cc.serverUser.Nick,
					}).ToMessage()
				}

			case UserCmd:
				if cc.state == clientStateRegistering {
					cc.state = clientStateAwaitingNick
				} else if cc.state == clientStateAwaitingUser {
					// Finish registration if we already have the NICK
					cc.state = clientStateRegistered
					cc.finishRegistration()
				}

			case PingCmd:
				var pingToken string
				if len(msg.Params) > 0 {
					pingToken = msg.Params[0]
				}

				cc.outgoingMessages <- (&Pong{
					ServerName: cc.config.ServerName,
					Token:      pingToken,
				}).ToMessage()

			case JoinCmd:
				channels := strings.Split(msg.Params[0], ",")

				for _, channelName := range channels {
					// Slack channel names are forcibly lowercased...RIP casemapping
					channelName = strings.ToLower(channelName)

					// Ignore if we've already joined this channel (to avoid sending WHO/NAMES again)
					if _, found := cc.joinedChans[channelName]; found {
						continue SelectLoop
					}

					// TODO join the channel on the Slack-side too
					cc.handleChannelJoined(channelName)
				}

			case PartCmd:
				// Slack channel names are forcibly lowercased...RIP casemapping
				channelName := strings.ToLower(msg.Params[0])

				// Ignore if we're not in this channel
				if _, found := cc.joinedChans[channelName]; !found {
					continue SelectLoop
				}

				// TODO part the channel on the Slack-side too
				cc.handleChannelParted(channelName)

			case ModeCmd:
				if len(msg.Params) < 1 || msg.Params[0][0] != '#' {
					// For Slack we only want to handle querying channel modes...
					continue
				}

				channelName := msg.Params[0]
				channelModes := "+nt"
				if cc.stateProvider.GetChannelPrivate(channelName) {
					channelModes += "sp"
				}
				cc.outgoingMessages <- cc.reply(NumericReply{
					Code:   RPL_CHANNELMODEIS,
					Params: []string{channelName, channelModes},
				})

				ctime := cc.stateProvider.GetChannelCTime(channelName)
				cc.outgoingMessages <- cc.reply(NumericReply{
					Code:   RPL_CREATIONTIME,
					Params: []string{channelName, fmt.Sprintf("%v", ctime.Unix())},
				})

			case TopicCmd:
				// nolint: megacheck
				if len(msg.Params) == 1 {
					channelName := msg.Params[0]
					topic := cc.stateProvider.GetChannelTopic(channelName)
					cc.sendChannelTopic(channelName, topic)
				} else {
					// TODO implement setting the topic
				}

			case WhoCmd:
				if len(msg.Params) < 1 {
					// Technically this is allowed but we'll just ignore it.
					continue
				}

				channelName := msg.Params[0]
				users := cc.stateProvider.GetChannelUsers(channelName)
				for _, m := range WholistAsNumerics(users, channelName, cc.config.ServerName) {
					cc.outgoingMessages <- cc.reply(*m)
				}

			case WhoisCmd:
				whoisNick := msg.Params[0]
				whoisUser := cc.stateProvider.GetUserFromNick(whoisNick)

				// Check for zero value, reply with no such user in that case.
				if whoisUser.Nick == "" {
					cc.outgoingMessages <- cc.reply(*ErrNoSuchNick(whoisNick))
					continue
				}

				for _, m := range WhoisAsNumerics(whoisUser) {
					cc.outgoingMessages <- cc.reply(*m)
				}
			}

		}
	}
}

// postProcessClientMessage modifies the message for consumption by the client
// adding targets or swapping things as needed
func (cc *clientConnection) postProcessClientMessage(m *Message) *Message {
	messagable, err := ParseMessage(m)
	if err != nil {
		return m
	}

	switch aMessagable := messagable.(type) {
	case *Privmsg:
		// We need to change a few fields if this message is a self message
		if aMessagable.IsFromSelf() {
			// If this was for a channel message, then we need to add
			// the self user as the From field
			if aMessagable.IsTargetChannel() {
				retMessage := aMessagable
				retMessage.From = cc.clientUser
				return retMessage.ToMessage()
			}

			// If not, this is a direct message, in which case we need to set
			// the From to the target user, set the target to the self user
			// and add text indicating that we were the original sender
			retMessage := aMessagable
			targetNick := aMessagable.Target
			targetUser := cc.stateProvider.GetUserFromNick(targetNick)

			retMessage.From = targetUser
			retMessage.Target = cc.clientUser.Nick
			retMessage.Message = "[" + cc.clientUser.Nick + "] " + retMessage.Message
			return retMessage.ToMessage()
		}
	default:
		return m
	}

	return m
}

func (cc *clientConnection) handleConnOutput() {
	for {
		select {
		case <-cc.shutdown:
			return

		case message := <-cc.outgoingMessages:
			if cc.state == clientStateRegistered {
				fmt.Fprintln(cc.conn, cc.postProcessClientMessage(message).String())
			}
		}
	}
}

func (cc *clientConnection) reply(reply NumericReply) *Message {
	reply.ServerName = cc.config.ServerName
	reply.Target = cc.clientUser.Nick
	return reply.ToMessage()
}

func (cc *clientConnection) sendWelcome() {
	messages := []*Message{
		cc.reply(NumericReply{
			Code:   RPL_WELCOME,
			Params: []string{fmt.Sprintf("Welcome to the tanya Slack IRC Gateway %v", cc.clientUser)},
		}),
		cc.reply(NumericReply{
			Code:   RPL_YOURHOST,
			Params: []string{"Your host is tanya, running SalarymanOS 9.0"},
		}),
		cc.reply(NumericReply{
			Code: RPL_ISUPPORT,
			Params: []string{
				"MAP",
				"SILENCE=15",
				"WALLCHOPS",
				"WALLVOICES",
				"USERIP",
				"CPRIVMSG",
				"CNOTICE",
				"MODES=6",
				"MAXCHANNELS=100",
				"SAFELIST",
				"are supported by this server",
			},
		}),
		cc.reply(NumericReply{
			Code: RPL_ISUPPORT,
			Params: []string{
				"NICKLEN=32",
				"TOPICLEN=160",
				"AWAYLEN=160",
				"CHANTYPES=#",
				"PREFIX=(ov)@+",
				"CHANMODES=b,k,l,rimnpst",
				"CASEMAPPING=rfc1459", // this is an especially egregious lie who even does rfc1459 casemapping
				"are supported by this server",
			},
		}),
	}

	for _, m := range messages {
		cc.outgoingMessages <- m
	}
	for _, m := range MOTDAsNumerics(cc.config.MOTD) {
		cc.outgoingMessages <- cc.reply(*m)
	}

	for _, channelName := range cc.stateProvider.GetJoinedChannels() {
		cc.handleChannelJoined(channelName)
	}
}

func (cc *clientConnection) sendChannelTopic(channelName string, topic ChannelTopic) {
	cc.outgoingMessages <- cc.reply(NumericReply{
		Code:   RPL_TOPIC,
		Params: []string{channelName, topic.Topic},
	})

	setBy := cc.config.ServerName
	if topic.SetBy != "" {
		setBy = topic.SetBy
	}
	cc.outgoingMessages <- cc.reply(NumericReply{
		Code:   RPL_TOPIC_WHOTIME,
		Params: []string{channelName, setBy, fmt.Sprintf("%v", topic.SetAt.Unix())},
	})
}

func (cc *clientConnection) handleChannelJoined(channelName string) {
	exists := cc.stateProvider.ChannelExists(channelName)
	if !exists {
		cc.outgoingMessages <- cc.reply(*ErrNoSuchChannel(channelName))
		return
	}

	topic := cc.stateProvider.GetChannelTopic(channelName)
	users := cc.stateProvider.GetChannelUsers(channelName)

	cc.joinedChans[channelName] = struct{}{}
	cc.sendChannelJoinedResponse(channelName, topic, users)
}

func (cc *clientConnection) sendChannelJoinedResponse(channelName string, topic ChannelTopic, users []User) {
	joinResponse := (&Join{
		User:    cc.clientUser,
		Channel: channelName,
	}).ToMessage()
	cc.outgoingMessages <- joinResponse

	cc.sendChannelTopic(channelName, topic)

	for _, m := range NamelistAsNumerics(users, channelName) {
		cc.outgoingMessages <- cc.reply(*m)
	}
}

func (cc *clientConnection) handleChannelParted(channelName string) {
	delete(cc.joinedChans, channelName)

	partResponse := (&Part{
		User:    cc.clientUser,
		Channel: channelName,
	}).ToMessage()
	cc.outgoingMessages <- partResponse
}
