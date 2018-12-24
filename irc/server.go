package irc

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Server represents the IRC server listener for bridging IRC clients to Slack
// and fanning out Slack events as necessary
type Server struct {
	clientConnections map[net.Addr]*clientConnection
	stopChan          <-chan interface{}

	initOnce sync.Once
	initChan chan interface{}

	selfUser      User
	config        *Config
	stateProvider ServerStateProvider

	sync.RWMutex
}

// ServerMessage is a message from a client to the server, indexed by the
// remote address of the connection
type ServerMessage struct {
	message Messagable
	cAddr   net.Addr
}

// ChannelTopic represents a channel's set topic and metadata
type ChannelTopic struct {
	Topic string
	SetBy string
	SetAt time.Time
}

// NewServer creates a new IRC server
func NewServer(config *Config, stopChan <-chan interface{}, stateProvider ServerStateProvider) *Server {
	return &Server{
		clientConnections: make(map[net.Addr]*clientConnection),
		stopChan:          stopChan,

		initChan: make(chan interface{}),

		config:        config,
		stateProvider: stateProvider,
	}
}

// Listen for and accept incoming connections on the configured address.
func (s *Server) Listen() {
	addr, err := net.ResolveTCPAddr("tcp", s.config.ListenAddr)
	if err != nil {
		log.Fatal(err)
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	log.Printf("IRC server now listening on %v", addr)
	serverChan := make(chan *ServerMessage)
	go s.handleIncomingMessageRouting(serverChan)
	go s.waitForKillListener(l)

	for {
		conn, err := l.AcceptTCP()
		if err != nil {
			if strings.HasSuffix(err.Error(), "closed network connection") {
				// If we are trying to accept from a closed socket that means
				// the socket was closed from underneath us, so there's no point
				// logging
				break
			} else {
				log.Fatal(err)
			}
		}

		cc := newClientConnection(conn, &s.selfUser, s.config, s.stateProvider, serverChan, s.initChan)
		log.Printf("IRC client connected: %v", cc)

		s.Lock()
		s.clientConnections[conn.RemoteAddr()] = cc
		s.Unlock()

		go cc.handleConnInput()
		go cc.handleConnOutput()
		go s.waitForClientCleanup(cc)
	}
}

func (s *Server) waitForClientCleanup(cc *clientConnection) {
	<-cc.shutdown

	log.Printf("IRC client disconnected: %v", cc)

	s.Lock()
	delete(s.clientConnections, cc.conn.RemoteAddr())
	s.Unlock()
}

func (s *Server) waitForKillListener(l *net.TCPListener) {
	<-s.stopChan
	l.Close()

	// First grab the lock and grab the active connections
	conns := make([]*clientConnection, 0)
	s.RLock()
	for _, conn := range s.clientConnections {
		conns = append(conns, conn)
	}
	s.RUnlock()

	// Now try to close them. This list could be stale, but it won't
	// cause any deadlocks
	for _, conn := range conns {
		close(conn.shutdown)
	}
}

func (s *Server) handleIncomingMessageRouting(incomingMessages <-chan *ServerMessage) {
	for {
		select {
		case <-s.stopChan:
			return
		case msg := <-incomingMessages:
			// Do not send the message back to the originator of the messages
			handleIncomingMessage(msg.message, s.stateProvider)

			s.RLock()
			for addr, conn := range s.clientConnections {
				if addr != msg.cAddr {
					conn.outgoingMessages <- msg.message.ToMessage()
				}
			}
			s.RUnlock()
		}
	}
}

func handleIncomingMessage(msg Messagable, ssp ServerStateProvider) {
	switch m := msg.(type) {
	case *Privmsg:
		ssp.SendPrivmsg(m)
	}
}

// HandleOutgoingMessageRouting handles fanning out IRC messages generated from Slack events
func (s *Server) HandleOutgoingMessageRouting(outgoingMessages <-chan *Message) {
	for {
		message := <-outgoingMessages

		s.RLock()
		for _, v := range s.clientConnections {
			v.outgoingMessages <- message
		}
		s.RUnlock()
	}
}

// HandleConnectBurst handles the initial burst of data from a
// freshly established Slack connection.
//
// Sets the IRC user info for the gateway user and informs clients.
func (s *Server) HandleConnectBurst(selfUser User) {
	oldSelfUser := s.selfUser
	s.selfUser = selfUser

	if oldSelfUser != selfUser {
		nickChangeMessage := (&Nick{
			From:    oldSelfUser,
			NewNick: selfUser.Nick,
		}).ToMessage()

		s.RLock()
		for _, v := range s.clientConnections {
			v.outgoingMessages <- nickChangeMessage
		}
		s.RUnlock()
	}

	s.initOnce.Do(func() { close(s.initChan) })
}

// HandleChannelJoined handles a Slack-initiated channel membership change event.
// This special signalling is necessary due to the extra data (NAMES/topic) which
// needs to be sent by the IRC server on join.
func (s *Server) HandleChannelJoined(channelName string) {
	topic := s.stateProvider.GetChannelTopic(channelName)
	users := s.stateProvider.GetChannelUsers(channelName)

	s.RLock()
	for _, v := range s.clientConnections {
		v.sendChannelJoinedResponse(channelName, topic, users)
	}
	s.RUnlock()
}

// ServerStateProvider contains methods used by the IRC server to answer
// client queries about channels and their members.
type ServerStateProvider interface {
	GetChannelUsers(channelName string) []User

	GetChannelTopic(channelName string) ChannelTopic

	GetChannelCTime(channelName string) time.Time

	GetJoinedChannels() []string

	SendPrivmsg(privMsg *Privmsg)

	GetUserFromNick(nick string) User
}
