package main

import (
	"bet/cli"
	"bet/core"
	"bet/core/commands"
	"bet/core/db"
	"bet/core/events"
	"bet/state"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Command interface {
	Command() *discordgo.ApplicationCommand
	Interaction(s *discordgo.Session, i *discordgo.InteractionCreate)
}

func main() {
	// Set up structured logging
	nowStr := time.Now().Format("060102_150405")
	logFile, err := os.Create(fmt.Sprintf("bet_%s.log", nowStr))
	if err != nil {
		fmt.Println("error creating a log file: %v", err)
		return
	}
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: cli.LogLevel}))
	slog.SetDefault(logger)

	// Load environment configuration
	environment, err := LoadEnvironemnt()
	if err != nil {
		slog.Error(fmt.Sprintf("error loading environment yaml: %s", err))
		return
	}

	// Start a discord bot session, so handlers can be registered.
	dg, err := discordgo.New("Bot " + environment.Token)
	if err != nil {
		slog.Error(fmt.Sprintf("error creating discord bot: %v", err))
		return
	}

	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info(fmt.Sprintf("Logged in as %s", r.User.String()))
		fmt.Printf("Logged in as %s\n", r.User.String())
	})

	// Open a database connection.
	slog.Info(environment.DbName)
	database, err := db.Open(environment.DbName)
	if err != nil {
		slog.Error(fmt.Sprintf("could not open databse: %s", err))
		return
	}
	defer database.Close()

	// Create the core.
	core := core.New(database, dg)
	if core == nil {
		slog.Error("could not create core, exiting")
		return
	}

	// Create Events/Updaters/State objects.
	// _ = updater.NewShinyUpdater(core, dg)
	l, err := state.NewListener(fmt.Sprintf("%s:%d", environment.Host, environment.Port), environment.PostAcl)
	if err != nil {
		slog.Error(fmt.Sprintf("err creating http server: %s", err))
		return
	}
	defer l.Close()
	shinyEvent := events.NewShinyEvent(core, environment.DiscordChannel)
	if err := core.RegisterEvent("shiny", shinyEvent); err != nil {
		slog.Error(fmt.Sprintf("err registering event: %s", err))
		return
	}
	l.Register(shinyEvent)
	antiEvent := events.NewAntiShinyEvent(core, environment.DiscordChannel)
	if err := core.RegisterEvent("anti", antiEvent); err != nil {
		slog.Error(fmt.Sprintf("err registering event: %s", err))
		return
	}
	l.Register(antiEvent)

	// Command initialization and registration.
	cs := map[string]Command{
		"balance":     &commands.BalanceCommand{Core: core},
		"bet":         &commands.BetCommand{Core: core},
		"leaderboard": &commands.LeaderboardCommand{Core: core},
		"bets":        &commands.ListBetsCommand{Core: core},
		"donate":      &commands.DonateCommand{Core: core},
		"ledger":      &commands.LedgerCommand{Core: core},
		"soon":        &commands.SoonCommand{Core: core},
	}
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error(fmt.Sprintf("recovering from panic in discord command handler: %s", r))
			}
		}()
		if h, ok := cs[i.ApplicationCommandData().Name]; ok {
			h.Interaction(s, i)
		}
	})
	commandList := make([]*discordgo.ApplicationCommand, 0, len(cs))
	for _, c := range cs {
		commandList = append(commandList, c.Command())
	}
	registeredCommands, err := dg.ApplicationCommandBulkOverwrite(environment.AppId, environment.DiscordServer, commandList)
	if err != nil {
		slog.Error(fmt.Sprintf("Failed to bulk register commands: %v", err))
	}

	// Open the session to start the bot running.
	err = dg.Open()
	if err != nil {
		slog.Error(fmt.Sprintf("Could not open session: %s", err))
		return
	}
	defer dg.Close()

	go cli.Loop()

	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":2112", nil)

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)
	<-sigch

	for _, c := range registeredCommands {
		if err := dg.ApplicationCommandDelete(environment.AppId, environment.DiscordServer, c.ID); err != nil {
			slog.Error(fmt.Sprintf("error removing command: %s", err))
		}
	}
}
