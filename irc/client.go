package irc

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
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

	state clientState

	outgoingMessages chan *Message
	serverChan       chan<- *ServerMessage
	shutdown         chan interface{}
}

func newClientConnection(
	conn *net.TCPConn,
	user *User,
	config *Config,
	stateProvider ServerStateProvider,
	serverChan chan *ServerMessage,
) *clientConnection {
	ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

	return &clientConnection{
		conn:          conn,
		config:        config,
		stateProvider: stateProvider,

		clientUser: User{Nick: "*", Ident: user.Ident, Host: ip},
		serverUser: user,

		outgoingMessages: make(chan *Message),
		serverChan:       serverChan,
		shutdown:         make(chan interface{}),
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

	s := bufio.NewScanner(cc.conn)

SelectLoop:
	for {
		select {
		case <-cc.shutdown:
			return

		default:
			if !s.Scan() {
				if err := s.Err(); err == nil {
					// Client conn hit an EOF
					close(cc.shutdown)
				}

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

				cc.serverChan <- &ServerMessage{
					message: ParseMessage(msg),
					cAddr:   cc.conn.RemoteAddr(),
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
					for _, joinedChannel := range cc.stateProvider.GetJoinedChannels() {
						if channelName == joinedChannel {
							continue SelectLoop
						}
					}

					// TODO make this actually join if we're not already a part of the channel
					cc.handleChannelJoined(channelName)
				}

			case PartCmd:
				// TODO make this do more than just echo
				msg.Prefix = cc.clientUser.String()
				cc.outgoingMessages <- msg

			case ModeCmd:
				if len(msg.Params) < 1 || msg.Params[0][0] != '#' {
					// For Slack we only want to handle querying channel modes...
					// And they'll always be just +nt
					continue
				}

				cc.outgoingMessages <- cc.reply(NumericReply{
					Code:   RPL_CHANNELMODEIS,
					Params: []string{msg.Params[0], "+nt"},
				})

				ctime := cc.stateProvider.GetChannelCTime(msg.Params[0])
				cc.outgoingMessages <- cc.reply(NumericReply{
					Code:   RPL_CREATIONTIME,
					Params: []string{msg.Params[0], fmt.Sprintf("%v", ctime.Unix())},
				})

			case TopicCmd:
				// nolint: megacheck
				if len(msg.Params) == 1 {
					cc.sendChannelTopic(msg.Params[0])
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
			}

		}
	}
}

func (cc *clientConnection) handleConnOutput() {
	for {
		select {
		case <-cc.shutdown:
			return

		case message := <-cc.outgoingMessages:
			if cc.state == clientStateRegistered {
				fmt.Fprintln(cc.conn, message.String())
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

func (cc *clientConnection) sendChannelTopic(channelName string) {
	topic := cc.stateProvider.GetChannelTopic(channelName)
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
	joinResponse := (&Join{
		User:    cc.clientUser,
		Channel: channelName,
	}).ToMessage()
	cc.outgoingMessages <- joinResponse

	cc.sendChannelTopic(channelName)

	users := cc.stateProvider.GetChannelUsers(channelName)
	for _, m := range NamelistAsNumerics(users, channelName) {
		cc.outgoingMessages <- cc.reply(*m)
	}
}
