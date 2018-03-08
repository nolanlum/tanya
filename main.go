package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
)

func handleConn(c net.Conn) {
	defer c.Close()
	s := bufio.NewScanner(c)
	for s.Scan() {
		// Reads a line with s.Text() and echoes it back
		fmt.Fprintln(c, s.Text())
	}
}

func main() {
	l, err := net.Listen("tcp", ":6667")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go handleConn(conn)
	}
}
