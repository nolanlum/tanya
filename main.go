package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/nolanlum/tanya/gateway"
	"github.com/nolanlum/tanya/irc"
)

func killHandler(sigChan <-chan os.Signal, stopChan chan<- interface{}) {
	<-sigChan
	log.Println("stopping connections and goroutines")
	close(stopChan)
}

func slackUserToIRCUser(s *gateway.SlackUser) irc.User {
	return irc.User{Nick: s.Nick, Ident: s.SlackID}
}

func slackToPrivmsg(m *gateway.MessageEventData) *irc.Privmsg {
	return &irc.Privmsg{
		From:    slackUserToIRCUser(&m.From),
		Channel: m.Target,
		Message: m.Message,
	}
}

func slackToNick(n *gateway.NickChangeEventData) *irc.Nick {
	return &irc.Nick{
		From:    slackUserToIRCUser(&n.From),
		NewNick: n.NewNick,
	}
}

func writeMessageLoop(
	recvChan <-chan *gateway.SlackEvent,
	sendChan chan<- *irc.Message,
	stopChan <-chan interface{},
	server *irc.Server,
) {
Loop:
	for {
		select {
		case <-stopChan:
			break Loop
		case msg := <-recvChan:
			switch msg.EventType {
			case gateway.SlackConnectedEvent:
				// This is a state-changing event. Not 100% sure the main goroutine
				// should be handling it but it doesn't make sense to have a separate
				// server goroutine just for reconnected events, nor does it make sense
				// to multiplex it onto sendChan.
				b := msg.Data.(*gateway.SlackConnectedEventData)
				server.HandleConnectBurst(slackUserToIRCUser(b.UserInfo))
			case gateway.MessageEvent:
				p := slackToPrivmsg(msg.Data.(*gateway.MessageEventData))
				sendChan <- p.ToMessage()
			case gateway.NickChangeEvent:
				n := slackToNick(msg.Data.(*gateway.NickChangeEventData))
				sendChan <- n.ToMessage()
			}
		}

	}
}

func main() {
	conf, err := LoadConfig()
	if err != nil {
		log.Fatal(err)
	}

	// Stop channel
	stopChan := make(chan interface{})

	// Setup our stop handling
	killSignalChan := make(chan os.Signal, 1)
	go killHandler(killSignalChan, stopChan)
	signal.Notify(killSignalChan, os.Interrupt)
	log.Println("starting tanya")

	slackIncomingChan := make(chan *gateway.SlackEvent)
	slackClient := gateway.NewSlackClient()
	slackClient.Initialize(conf.Slack.Token)
	if err != nil {
		log.Fatal(err)
	}
	go slackClient.Poop(&gateway.ClientChans{
		IncomingChan: slackIncomingChan,
		StopChan:     stopChan,
	})

	ircOutgoingChan := make(chan *irc.Message)
	ircServer := irc.NewServer(&conf.IRC, stopChan)
	go ircServer.Listen()
	go ircServer.HandleOutgoingMessageRouting(ircOutgoingChan)

	log.Println("tanya ready for connections")
	writeMessageLoop(slackIncomingChan, ircOutgoingChan, stopChan, ircServer)
	log.Println("tanya shutting down")
}
