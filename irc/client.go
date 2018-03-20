package irc

import (
	"bufio"
	"fmt"
	"log"
	"net"
)

type clientState int

const (
	clientStateRegistering clientState = iota
	clientStateRegistered
)

type clientConnection struct {
	conn   *net.TCPConn
	config *Config

	clientUser User
	serverUser *User

	state clientState

	outgoingMessages chan *Message
	shutdown         chan interface{}
}

func newClientConnection(conn *net.TCPConn, user *User, config *Config) *clientConnection {
	ip, _, _ := net.SplitHostPort(conn.RemoteAddr().String())

	return &clientConnection{
		conn:   conn,
		config: config,

		clientUser: User{Nick: "*", Ident: user.Ident, Host: ip},
		serverUser: user,

		outgoingMessages: make(chan *Message),
		shutdown:         make(chan interface{}),
	}
}

func (cc *clientConnection) String() string {
	return fmt.Sprintf("%v", cc.conn.RemoteAddr())
}

func (cc *clientConnection) handleConnInput() {
	defer func() {
		close(cc.shutdown)
		cc.conn.Close()
	}()

	s := bufio.NewScanner(cc.conn)
	for s.Scan() {
		// Reads a line with s.Text() and parses it as
		// an IRC message
		msgStr := s.Text()
		msg, err := StringToMessage(msgStr)

		// Pretty sure error handling shouldn't be this complicated
		if err != nil {
			if err == ErrMalformedIRCMessage {
				log.Printf("[%v] sent malformed IRC message: %v", cc, msgStr)
			} else if numeric, ok := err.(*NumericReply); ok {
				numeric.Target = cc.clientUser.Nick
				fmt.Fprintln(cc.conn, numeric.ToMessage().String())
			} else {
				log.Printf("[%v] error: %v", cc, err)
			}
			continue
		}

		switch msg.Cmd {
		case NickCmd:
			if cc.state == clientStateRegistering {
				cc.clientUser.Nick = msg.Params[0]
			} else {
				cc.outgoingMessages <- (&Nick{
					From:    User{Nick: msg.Params[0], Ident: cc.serverUser.Ident},
					NewNick: cc.serverUser.Nick,
				}).ToMessage()
			}

		case UserCmd:
			cc.state = clientStateRegistered

			// Set the client straight if its nick is wrong
			if cc.clientUser.Nick != cc.serverUser.Nick {
				cc.outgoingMessages <- (&Nick{
					From:    User{Nick: cc.clientUser.Nick, Ident: cc.serverUser.Ident},
					NewNick: cc.serverUser.Nick,
				}).ToMessage()
			}
			cc.clientUser = *cc.serverUser

			// Send the welcome messages
			cc.sendWelcome()

		case PingCmd:
			var pingToken string
			if len(msg.Params) > 0 {
				pingToken = msg.Params[0]
			}

			cc.outgoingMessages <- (&Pong{
				ServerName: cc.config.ServerName,
				Token:      pingToken,
			}).ToMessage()

		case JoinCmd, PartCmd:
			// TODO make this do more than just echo
			msg.Prefix = cc.clientUser.String()
			cc.outgoingMessages <- msg
		}
	}
}

func (cc *clientConnection) handleConnOutput() {
	for {
		select {
		case <-cc.shutdown:
			return

		case message := <-cc.outgoingMessages:
			fmt.Fprintln(cc.conn, message.String())
		}
	}
}

func (cc *clientConnection) reply(code NumericCommand, params []string) *Message {
	return (&NumericReply{
		Code:   code,
		Target: cc.clientUser.Nick,
		Params: params,
	}).ToMessage()
}

func (cc *clientConnection) sendWelcome() {
	messages := []*Message{
		cc.reply(
			RPL_WELCOME, []string{fmt.Sprintf("Welcome to the tanya Slack IRC Gateway %v", cc.clientUser)}),
		cc.reply(RPL_YOURHOST, []string{"Your host is tanya, running SalarymanOS 9.0"}),
	}

	for _, m := range messages {
		cc.outgoingMessages <- m
	}
	for _, m := range MOTDAsNumerics(cc.clientUser.Nick, cc.config.MOTD) {
		cc.outgoingMessages <- m.ToMessage()
	}
}
