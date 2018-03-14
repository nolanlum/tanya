package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"

	"github.com/nolanlum/tanya/gateway"
	"github.com/nolanlum/tanya/irc"
)

func killHandler(sigChan <-chan os.Signal, mLoopChan chan<- bool, slackChan chan<- bool, ircChan chan<- bool) {
	<-sigChan
	log.Println("stopping connections and goroutines")
	slackChan <- true
	ircChan <- true
	mLoopChan <- true	
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

func writeMessageLoop(recvChan <-chan *gateway.SlackEvent, sendChan chan<- *irc.Message, stopChan <-chan bool) {
	Loop: for {
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


	stopSlackChan := make(chan bool)
	stopIrcChan := make(chan bool)
	stopMessageLoopChan := make(chan bool)

	// Setup our stop handling
	killSignalChan := make(chan os.Signal, 1)
	go killHandler(killSignalChan, stopMessageLoopChan, stopSlackChan, stopIrcChan)
	signal.Notify(killSignalChan, os.Kill)
	// Windows does not support the Interrupt signal
	if runtime.GOOS != "windows" {
		signal.Notify(killSignalChan, os.Interrupt)
	}

	log.Println("starting tanya")

	slackIncomingChan := make(chan *gateway.SlackEvent)
	slackClient := gateway.NewSlackClient()
	slackClient.Initialize(conf.Slack.Token)
	go slackClient.Poop(&gateway.ClientChans{
		IncomingChan: slackIncomingChan,
		StopChan: stopSlackChan,
	})

	ircOutgoingChan := make(chan *irc.Message)
	ircServer := irc.NewServer(stopIrcChan)
	go ircServer.Listen(&net.TCPAddr{Port: 6667})
	go ircServer.HandleOutgoingMessageRouting(ircOutgoingChan)

	log.Println("tanya ready for connections")
	writeMessageLoop(slackIncomingChan, ircOutgoingChan, stopMessageLoopChan)
	log.Println("tanya shutting down")
}
