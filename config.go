package main

import (
	"io/ioutil"

	"github.com/BurntSushi/toml"
)

type slack struct {
	Token string
}

// Config holds configuration data for Tanya
type Config struct {
	Slack slack
}

// ParseConfig reads a ./config.toml file and retruns a parsed config
func ParseConfig() (*Config, error) {
	tomlData, err := ioutil.ReadFile("config.toml")
	if err != nil {
		return nil, err
	}

	var conf Config
	if _, err = toml.Decode(string(tomlData), &conf); err != nil {
		return nil, err
	}

	return &conf, nil
}
