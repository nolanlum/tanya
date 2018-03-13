package main

import (
	"bufio"
	"fmt"
	"log"
	"net"

	"github.com/nolanlum/tanya/gateway"
	"github.com/nolanlum/tanya/irc"
)

type ircMessage struct {
	thing string
}

type ircServer struct {
	Channels []string
	Nick     string
	User     string
}

func slackToPrivmsg(m *gateway.MessageEventData) *irc.Privmsg {
	return &irc.Privmsg{
		From:    m.Nick,
		Channel: m.Target,
		Message: m.Message,
	}
}

func slackToNick(n *gateway.NickChangeEventData) *irc.Nick {
	return &irc.Nick{
		From:    n.OldNick,
		NewNick: n.NewNick,
	}
}

func handleConn(c net.Conn) {
	defer c.Close()
	s := bufio.NewScanner(c)
	for s.Scan() {
		// Reads a line with s.Text() and parses it as
		// an IRC message
		msg, err := irc.StringToMessage(s.Text())
		if err != nil {
			// TODO: make this an actual IRC error
			fmt.Fprintln(c, "malformed IRC message")
		} else {
			fmt.Fprintln(c, msg)
		}
	}
}

func writeMessageLoop(c net.Conn, recvChan <-chan *gateway.SlackEvent) {
	for {
		msg := <-recvChan
		switch msg.EventType {
		case gateway.MessageEvent:
			p := slackToPrivmsg(msg.Data.(*gateway.MessageEventData))
			fmt.Fprintln(c, p.ToMessage().String())
		case gateway.NickChangeEvent:
			n := slackToNick(msg.Data.(*gateway.NickChangeEventData))
			fmt.Fprintln(c, n.ToMessage().String())
		}
	}
}

func main() {
	l, err := net.Listen("tcp", ":6667")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	conf, err := ParseConfig()
	if err != nil {
		log.Fatal(err)
	}

	stopChan := make(chan bool)
	recvChan := make(chan *gateway.SlackEvent)
	slackClient := gateway.NewSlackClient()
	slackClient.Initialize(conf.Slack.Token)
	go slackClient.Poop(&gateway.ClientChans{
		StopChan:     stopChan,
		IncomingChan: recvChan,
	})
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
		go writeMessageLoop(conn, recvChan)
	}
}
