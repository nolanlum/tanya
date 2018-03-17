package irc

// Config holds configurable parameters for the IRC server
type Config struct {
	ListenAddr string

	MOTD string
}
