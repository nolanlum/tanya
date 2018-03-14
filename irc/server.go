package irc

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"sync"
)

type clientConnection struct {
	conn *net.TCPConn

	outgoingMessages chan *Message
	shutdown         chan interface{}
}

func newClientConnection(conn *net.TCPConn) *clientConnection {
	return &clientConnection{
		conn: conn,

		outgoingMessages: make(chan *Message),
		shutdown:         make(chan interface{}),
	}
}

func (cc *clientConnection) handleConnInput() {
	defer func() {
		close(cc.shutdown)
		cc.conn.Close()
	}()

	s := bufio.NewScanner(cc.conn)
	for s.Scan() {
		// Reads a line with s.Text() and parses it as
		// an IRC message
		msg, err := StringToMessage(s.Text())
		if err != nil {
			// TODO: make this an actual IRC error
			fmt.Fprintln(cc.conn, "malformed IRC message")
		} else {
			fmt.Fprintln(cc.conn, msg)
		}
	}
}

func (cc *clientConnection) handleConnOutput() {
	for {
		select {
		case <-cc.shutdown:
			return

		case message := <-cc.outgoingMessages:
			fmt.Fprintln(cc.conn, message.String())
		}
	}
}

// Server represents the IRC server listener for bridging IRC clients to Slack
// and fanning out Slack events as necessary
type Server struct {
	clientConnections map[net.Addr]*clientConnection

	sync.RWMutex
}

// NewServer creates a new IRC server
func NewServer() *Server {
	return &Server{
		clientConnections: make(map[net.Addr]*clientConnection),
	}
}

// Listen for and accept incoming connections on the given address.
func (s *Server) Listen(addr *net.TCPAddr) {
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	for {
		conn, err := l.AcceptTCP()
		if err != nil {
			log.Fatal(err)
		}

		log.Printf("IRC client connected: %v", conn.RemoteAddr())
		cc := newClientConnection(conn)

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

	log.Printf("IRC client disconnected: %v", cc.conn.RemoteAddr())

	s.Lock()
	delete(s.clientConnections, cc.conn.RemoteAddr())
	s.Unlock()
}

// Handle fanning out IRC messages generated from Slack events
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