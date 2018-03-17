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

func writeMessageLoop(recvChan <-chan *gateway.SlackEvent, sendChan chan<- *irc.Message, stopChan <-chan interface{}) {
Loop:
	for {
		select {
		case <-stopChan:
			break Loop
		case msg := <-recvChan:
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
	slackUser, err := slackClient.Initialize(conf.Slack.Token, conf.Slack.UserID)
	if err != nil {
		log.Fatal(err)
	}
	go slackClient.Poop(&gateway.ClientChans{
		IncomingChan: slackIncomingChan,
		StopChan:     stopChan,
	})
	log.Printf("tanya logged into slack as %+v\n", slackUser)

	ircOutgoingChan := make(chan *irc.Message)
	ircServer := irc.NewServer(&conf.IRC, stopChan)
	ircServer.SetSelfInfo(slackUserToIRCUser(slackUser))
	go ircServer.Listen()
	go ircServer.HandleOutgoingMessageRouting(ircOutgoingChan)

	log.Println("tanya ready for connections")
	writeMessageLoop(slackIncomingChan, ircOutgoingChan, stopChan)
	log.Println("tanya shutting down")
}
