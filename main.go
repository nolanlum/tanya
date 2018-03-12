package main

import (
	"bufio"
	"fmt"
	"log"
	"net"

	"github.com/nolanlum/tanya/gateway"
	"github.com/nolanlum/tanya/irc"
)

type ircMessage struct {
	thing string
}

type ircServer struct {
	Channels []string
	Nick     string
	User     string
}

func slackToUtterance(m *gateway.Message) *irc.Utterance {
	return &irc.Utterance{
		From: m.Nick,
		Channel: m.Channel,
		Message: m.Data,
	}
}

func handleConn(c net.Conn) {
	defer c.Close()
	s := bufio.NewScanner(c)
	for s.Scan() {
		// Reads a line with s.Text() and parses it as
		// an IRC message
		msg, err := irc.StringToMessage(s.Text())
		if err != nil {
			// TODO: make this an actual IRC error
			fmt.Fprintln(c, "malformed IRC message")
		} else {
			fmt.Fprintln(c, msg)
		}
	}
}

func writeMessageLoop(c net.Conn, recvChan <-chan *gateway.Message) {
	for {
		msg := <- recvChan
		u := slackToUtterance(msg)
		fmt.Fprintln(c, u.ToMessage().String())
	}
}

func main() {
	l, err := net.Listen("tcp", ":6667")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()

	conf, err := ParseConfig()
	if err != nil {
		log.Fatal(err)
	}

	stopChan := make(chan bool)
	recvChan := make(chan *gateway.Message)
	go gateway.Poop(
		conf.Slack.Token,
		&gateway.ClientChans{
			StopChan: stopChan,
			SendChan: recvChan,
		})
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
		go writeMessageLoop(conn, recvChan)
	}
}
