package main

import (
	"bet/core"
	"bet/core/commands"
	"bet/core/db"
	"bet/core/events"
	"bet/state"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
)

type Command interface {
	Command() *discordgo.ApplicationCommand
	Interaction(s *discordgo.Session, i *discordgo.InteractionCreate)
}

func main() {
	// Load environment configuration
	environment, err := LoadEnvironemnt()
	if err != nil {
		log.Printf("error loading environment yaml: %s", err)
		return
	}

	// Start a discord bot session, so handlers can be registered.
	dg, err := discordgo.New("Bot " + environment.Token)
	if err != nil {
		log.Printf("error creating discord bot: %v", err)
		return
	}

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("Logged in as %s", r.User.String())
	})

	// Open a database connection.
	log.Println(environment.DbName)
	database, err := db.Open(environment.DbName)
	if err != nil {
		log.Printf("could not open databse: %s", err)
		return
	}
	defer database.Close()

	// Create the core.
	core := core.New(database)
	if core == nil {
		log.Println("could not create core, exiting")
		return
	}

	// Create Events/Updaters/State objects.
	// _ = updater.NewShinyUpdater(core, dg)
	l, err := state.NewListener(fmt.Sprintf("%s:%d", environment.Host, environment.Port), environment.PostAcl)
	if err != nil {
		log.Printf("err creating http server: %s", err)
		return
	}
	defer l.Close()
	shinyEvent := events.NewShinyEvent(core, dg, environment.DiscordChannel)
	if err := core.RegisterEvent("shiny", shinyEvent); err != nil {
		log.Printf("err registering event: %s", err)
		return
	}
	l.Register(shinyEvent)

	// Command initialization and registration.
	cs := map[string]Command{
		"balance":     &commands.BalanceCommand{Core: core},
		"bet":         &commands.BetCommand{Core: core},
		"leaderboard": &commands.LeaderboardCommand{Core: core},
		"bets":        &commands.ListBetsCommand{Core: core},
		"donate":      &commands.DonateCommand{Core: core},
		"ledger":      &commands.LedgerCommand{Core: core},
	}
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if h, ok := cs[i.ApplicationCommandData().Name]; ok {
			h.Interaction(s, i)
		}
	})
	// TODO: try to use bulk command overwrite to create all the commands?  That's
	// my best guess for /bets doesn't show up.
	commandList := make([]*discordgo.ApplicationCommand, 0, len(cs))
	for _, c := range cs {
		commandList = append(commandList, c.Command())
		// if _, err := dg.ApplicationCommandCreate(environment.AppId, environment.DiscordServer, c.Command()); err != nil {
		// 	log.Printf("Failed to register command %s: %v", name, err)
		// }
	}
	registeredCommands, err := dg.ApplicationCommandBulkOverwrite(environment.AppId, environment.DiscordServer, commandList)
	if err != nil {
		log.Printf("Failed to bulk register commands: %v", err)
	}

	// Open the session to start the bot running.
	err = dg.Open()
	if err != nil {
		log.Printf("Could not open session: %s", err)
		return
	}
	defer dg.Close()

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)
	<-sigch

	for _, c := range registeredCommands {
		if err := dg.ApplicationCommandDelete(environment.AppId, environment.DiscordServer, c.ID); err != nil {
			log.Printf("error removing command: %s", err)
		}
	}
}
