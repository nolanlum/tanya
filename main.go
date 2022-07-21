package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/nolanlum/tanya/gateway"
	"github.com/nolanlum/tanya/irc"
)

var configPathFlag = flag.String("config", "config.toml", "path to config file")
var noGenFlag = flag.Bool("no-generate", false, "disables auto-generation of config files")
var debugFlag = flag.Bool("debug", false, "toggles Slack library debug mode (logs to stdout)")

func killHandler(sigChan <-chan os.Signal, stopChan chan<- interface{}) {
	<-sigChan
	log.Println("tanya shutting down, stopping connections and goroutines")
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
		Target:  m.Target,
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

func slackToJoin(j *gateway.JoinPartEventData) *irc.Join {
	return &irc.Join{
		User:    slackUserToIRCUser(&j.User),
		Channel: j.Target,
	}
}

func slackToPart(j *gateway.JoinPartEventData) *irc.Part {
	return &irc.Part{
		User:    slackUserToIRCUser(&j.User),
		Channel: j.Target,
		Message: "Leaving",
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
		log.Printf("%s error while querying user list for %v: channel_not_found", c.sc.Tag(), channelName)
		return nil
	}

	channelUsers, err := c.sc.GetChannelUsers(channel.SlackID)
	if err != nil {
		log.Printf("%s error while querying user list for %v: %v", c.sc.Tag(), channelName, err)
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
		log.Printf("%s error while querying topic for %v: channel_not_found", c.sc.Tag(), channelName)
		return
	}

	topic.Topic = c.sc.ParseMessageText(channel.Topic.Value)
	topic.Topic = strings.Replace(topic.Topic, "\n", " ", -1)
	topic.SetAt = channel.Topic.LastSet.Time()

	if channel.Topic.Creator != "" {
		setBy, err := c.sc.ResolveUser(channel.Topic.Creator)
		if err != nil {
			log.Printf("%s error while querying topic creator for %v: %v", c.sc.Tag(), channelName, err)
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
		log.Printf("%s error while querying ctime for %v: channel_not_found", c.sc.Tag(), channelName)
		return time.Time{}
	}

	return channel.Created
}

// GetChannelPrivate implements irc.ServerStateProvider.GetChannelPrivate
func (c *corpusCallosum) GetChannelPrivate(channelName string) bool {
	channel := c.sc.ResolveNameToChannel(channelName)
	if channel == nil {
		log.Printf("%s error while querying private flag for %v: channel_not_found", c.sc.Tag(), channelName)
		return false
	}

	return channel.Private
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

	// Don't bother sending anything on an empty message
	if len(privMsg.Message) == 0 {
		return
	}

	if privMsg.IsTargetChannel() {
		channel := c.sc.ResolveNameToChannel(privMsg.Target)
		err := c.sc.SendMessage(channel, privMsg.Message)
		if err != nil {
			log.Printf("%s error sending slack message: %v\n", c.sc.Tag(), err)
		}
	} else if privMsg.IsValidTarget() {
		slackUser := c.sc.ResolveNickToUser(privMsg.Target)
		if slackUser != nil {
			c.sc.SendDirectMessage(slackUser, privMsg.Message)
		}
	}
}

func (c *corpusCallosum) GetUserFromNick(nick string) irc.User {
	slackUser := c.sc.ResolveNickToUser(nick)
	if slackUser != nil {
		return slackUserToIRCUser(slackUser)
	}
	return irc.User{}
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
			case gateway.SelfJoinEvent:
				server.HandleChannelJoined(msg.Data.(*gateway.JoinPartEventData).Target)
			case gateway.JoinEvent:
				j := slackToJoin(msg.Data.(*gateway.JoinPartEventData))
				sendChan <- j.ToMessage()
			case gateway.PartEvent:
				p := slackToPart(msg.Data.(*gateway.JoinPartEventData))
				sendChan <- p.ToMessage()
			}
		}
	}
}

func launchGateway(conf *GatewayInstance, stopChan chan interface{}) {
	slackIncomingChan := make(chan *gateway.SlackEvent)
	slackClient := gateway.NewSlackClient()
	slackClient.Initialize(conf.Slack.Token, conf.Slack.Cookie, *debugFlag)

	go slackClient.Poop(&gateway.ClientChans{
		IncomingChan: slackIncomingChan,
		StopChan:     stopChan,
	})

	ircOutgoingChan := make(chan *irc.Message)
	ircStateProvider := &corpusCallosum{slackClient}
	ircServer := irc.NewServer(&conf.IRC, stopChan, ircStateProvider)
	go ircServer.Listen()
	go ircServer.HandleOutgoingMessageRouting(ircOutgoingChan)

	writeMessageLoop(slackIncomingChan, ircOutgoingChan, stopChan, ircServer)
}

func main() {
	flag.Parse()

	conf, err := LoadConfig(*configPathFlag, *noGenFlag)
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

	var wg sync.WaitGroup
	wg.Add(len(conf.Gateway))
	for _, g := range conf.Gateway {
		go func(g GatewayInstance) {
			launchGateway(&g, stopChan)
			wg.Done()
		}(g)
	}

	wg.Wait()
	log.Println("goodbye!")
}
