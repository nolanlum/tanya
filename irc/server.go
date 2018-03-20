package irc

import (
	"log"
	"net"
	"strings"
	"sync"
)

// Server represents the IRC server listener for bridging IRC clients to Slack
// and fanning out Slack events as necessary
type Server struct {
	clientConnections map[net.Addr]*clientConnection
	stopChan          <-chan interface{}

	selfUser User
	config   *Config

	sync.RWMutex
}

// NewServer creates a new IRC server
func NewServer(config *Config, stopChan <-chan interface{}) *Server {
	return &Server{
		clientConnections: make(map[net.Addr]*clientConnection),
		stopChan:          stopChan,

		config: config,
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

	for {
		go s.waitForKillListener(l)
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

		cc := newClientConnection(conn, &s.selfUser, s.config)
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
}
