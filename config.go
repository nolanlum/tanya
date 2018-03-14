package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/nolanlum/tanya/token"
)

type slack struct {
	Token string
}

// Config holds configuration data for Tanya
type Config struct {
	Slack slack
}

// LoadConfig parses a config if it exists, or generates a new one
func LoadConfig() (*Config, error) {
	tomlData, err := ioutil.ReadFile("config.toml")

	if os.IsNotExist(err) {
		fmt.Println("config.toml does not exist, generating one...")
		return initializeConfig()
	} else if err != nil {
		return nil, err
	}

	return parseConfig(string(tomlData))
}

// initializeConfig interactively generates and writes a config
func initializeConfig() (*Config, error) {
	var conf Config
	token, err := token.GetSlackToken()
	if err != nil {
		return nil, err
	}

	conf.Slack.Token = token

	fmt.Print("Writing config.toml...")
	f, err := os.Create("config.toml")
	if err != nil {
		return nil, err
	}
	if err := toml.NewEncoder(f).Encode(conf); err != nil {
		return nil, err
	}
	fmt.Println("Done")
	return &conf, nil
}

// parseConfig reads a toml string and returns a parsed config
func parseConfig(tomlData string) (*Config, error) {
	var conf Config
	if _, err := toml.Decode(tomlData, &conf); err != nil {
		return nil, err
	}

	return &conf, nil
}
