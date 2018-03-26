package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

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

func slackToTopic(t *gateway.TopicChangeEventData) *irc.Topic {
	return &irc.Topic{
		From:    slackUserToIRCUser(&t.From),
		Channel: t.Target,
		Topic:   t.NewTopic,
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

// GetChannelTopic implements irc.ServerStateProvider.GetChannelTopic
func (c *corpusCallosum) GetChannelTopic(channelName string) (topic irc.ChannelTopic) {
	channel := c.sc.ResolveNameToChannel(channelName)
	if channel == nil {
		log.Printf("error while querying topic for %v: channel_not_found", channelName)
		return
	}

	topic.Topic = c.sc.ParseMessageText(channel.Topic.Value)
	topic.Topic = strings.Replace(topic.Topic, "\n", " ", -1)
	topic.SetAt = channel.Topic.LastSet.Time()

	if channel.Topic.Creator != "" {
		setBy, err := c.sc.ResolveUser(channel.Topic.Creator)
		if err != nil {
			log.Printf("error while querying topic creator for %v: %v", channelName, err)
			return
		}
		topic.SetBy = setBy.Nick
	}

	return
}

// GetChannelCTime implements irc.ServerStateProvider.GetChannelCTime
func (c *corpusCallosum) GetChannelCTime(channelName string) time.Time {
	channel := c.sc.ResolveNameToChannel(channelName)
	if channel == nil {
		log.Printf("error while querying ctime for %v: channel_not_found", channelName)
		return time.Time{}
	}

	return channel.Created
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
			case gateway.TopicChangeEvent:
				t := slackToTopic(msg.Data.(*gateway.TopicChangeEventData))
				sendChan <- t.ToMessage()
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
