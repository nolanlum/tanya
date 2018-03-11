package main

import (
	"io/ioutil"

	"github.com/BurntSushi/toml"
)

type slack struct {
	Token string
}

type Config struct {
	Slack slack
}

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
