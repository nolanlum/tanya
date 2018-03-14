package main

import (
	"log"
	"net"

	"github.com/nolanlum/tanya/gateway"
	"github.com/nolanlum/tanya/irc"
)

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

func writeMessageLoop(recvChan <-chan *gateway.SlackEvent, sendChan chan<- *irc.Message) {
	for {
		msg := <-recvChan
		switch msg.EventType {
		case gateway.MessageEvent:
			p := slackToPrivmsg(msg.Data.(*gateway.MessageEventData))
			sendChan <- p.ToMessage()
		case gateway.NickChangeEvent:
			n := slackToNick(msg.Data.(*gateway.NickChangeEventData))
			sendChan <- n.ToMessage()
		}
	}
}

func main() {
	conf, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	slackIncomingChan := make(chan *gateway.SlackEvent)
	slackClient := gateway.NewSlackClient()
	slackClient.Initialize(conf.Slack.Token)
	go slackClient.Poop(&gateway.ClientChans{
		IncomingChan: slackIncomingChan,
	})

	ircOutgoingChan := make(chan *irc.Message)
	ircServer := irc.NewServer()
	go ircServer.Listen(&net.TCPAddr{Port: 6667})
	go ircServer.HandleOutgoingMessageRouting(ircOutgoingChan)

	writeMessageLoop(slackIncomingChan, ircOutgoingChan)
}
