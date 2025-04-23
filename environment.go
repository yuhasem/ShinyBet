package main

import (
	"os"

	"gopkg.in/yaml.v2"
)

// Environment is loaded from a .env YAML file for configurability.
type Enviroment struct {
	// Token is the discord bot token, which is used to log in as that bot.
	Token string
	// AppId is the discord bot application id, which is used to register
	// commands.
	AppId string
	// DbName is the name of the database to load from, following sqlite3
	// syntax.
	DbName string
	// Host is the host name to listen on.
	Host string
	// Port is the port number to listen on.  Together host and port represent
	// and address that a pokebot can send HTTP POST requests to to track when
	// shinies happen.
	Port int
	// PostAcl is a list of IP Addresses to accept POST messages from.  When the
	// list is empty ALL requests will be accepted..
	PostAcl []string
	// DiscordServer is the server to accept commands from.  It can be left
	// blank to accept commands from all servers.
	DiscordServer string
	// DiscordChannel is the channel to accept command from (UNIMPLEMENTED).
	DiscordChannel string
	// Events contains all the configuration for events.
	Events EventConfig
}

// EventConfig contains all the ways to configure which events are run.
type EventConfig struct {
	// Enables the shiny event
	EnableShiny bool
	// Enables the anti shiny event
	EnableAnti bool
}

func LoadEnvironemnt() (*Enviroment, error) {
	e := &Enviroment{}
	data, err := os.ReadFile(".env")
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, e); err != nil {
		return nil, err
	}
	return e, nil
}
