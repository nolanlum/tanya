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
	return irc.User{
		Nick:     s.Nick,
		Ident:    s.SlackID,
		Host:     "localhost",
		RealName: s.RealName,
	}
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

// this name specially chosen to trigger ATRAN
type corpusCallosum struct {
	sc *gateway.SlackClient
}

// GetChannelUsers implements irc.ServerStateProvider.GetChannelUsers
func (c *corpusCallosum) GetChannelUsers(channelName string) []irc.User {
	channel := c.sc.ResolveNameToChannel(channelName)
	if channel == nil {
		log.Printf("error while querying user list for %v: channel_not_found", channelName)
		return nil
	}

	channelUsers, err := c.sc.GetChannelUsers(channel.SlackID)
	if err != nil {
		log.Printf("error while querying user list for %v: %v", channelName, err)
		return nil
	}

	var users []irc.User
	for _, user := range channelUsers {
		users = append(users, slackUserToIRCUser(&user))
	}
	return users
}

// GetJoinedChannels implements irc.ServerStateProvider.GetJoinedChannels
func (c *corpusCallosum) GetJoinedChannels() []string {
	var channelNames []string
	for _, channel := range c.sc.GetChannelMemberships() {
		channelNames = append(channelNames, channel.Name)
	}
	return channelNames
}

// SendPrivmsg sends an IRC PRIVMSG through Slack resolving channels properly
func (c *corpusCallosum) SendPrivmsg(privMsg *irc.Privmsg) {
	// TODO: we should enforce that we are not sending PRIVMSGs from other people

	channel := c.sc.ResolveNameToChannel(privMsg.Channel)
	err := c.sc.SendMessage(channel, privMsg.Message)
	if err != nil {
		log.Printf("error sending slack message: %v\n", err)
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
	ircStateProvider := &corpusCallosum{slackClient}
	ircServer := irc.NewServer(&conf.IRC, stopChan, ircStateProvider)
	go ircServer.Listen()
	go ircServer.HandleOutgoingMessageRouting(ircOutgoingChan)

	log.Println("tanya ready for connections")
	writeMessageLoop(slackIncomingChan, ircOutgoingChan, stopChan, ircServer)
	log.Println("tanya shutting down")
}
