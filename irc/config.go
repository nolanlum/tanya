package irc

// Config holds configurable parameters for the IRC server
type Config struct {
	ServerName string
	ListenAddr string

	MOTD string
}

// SetDefaults overwrites config entries with their default values
func (c *Config) SetDefaults() {
	c.ServerName = "tanya"
	c.ListenAddr = ":6667"
}
