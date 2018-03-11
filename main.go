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

	go gateway.Poop(conf.Slack.Token)
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
	}
}
