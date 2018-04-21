package tracing

type Config struct {
	Enabled bool

	ServiceName     string
	ServiceHostPort string

	EndpointURL string
}

// SetDefaults overwrites config entries with their default values
func (c *Config) SetDefaults() {
	c.ServiceName = "tanya"
}
