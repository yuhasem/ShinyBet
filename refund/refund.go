package main

import (
	"bet/core"
	"bet/core/db"
	"bet/core/events"
	"bet/env"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/bwmarrin/discordgo"
)

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func main() {
	// Set up structured logging
	nowStr := time.Now().Format("060102_150405")
	logFile, err := os.Create(fmt.Sprintf("refund_%s.log", nowStr))
	if err != nil {
		fmt.Println("error creating a log file: %v", err)
		return
	}
	logger := slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	// Load environment configuration
	environment, err := env.LoadRefundEnvironment()
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
	core := core.New(database, dg, realClock{})
	if core == nil {
		slog.Error("could not create core, exiting")
		return
	}
	defer core.Close()

	// Open the session to start the bot running.
	err = dg.Open()
	if err != nil {
		slog.Error(fmt.Sprintf("Could not open session: %s", err))
		return
	}
	defer dg.Close()

	refund(core, environment)

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, os.Interrupt)
	<-sigch
}

// TODO: This just considers item events for now because that was the thing that
// broke.  It would be nice to have this work for other event types.
func refund(c *core.Core, environment *env.RefundEnv) {
	EventID := environment.Event.ID
	OpenTS, err := time.Parse(time.DateTime, environment.Event.Start)
	if err != nil {
		slog.Error(fmt.Sprintf("parse start: %v", err))
		return
	}
	CloseTS, err := time.Parse(time.DateTime, environment.Event.End)
	if err != nil {
		slog.Error(fmt.Sprintf("parse end: %v", err))
		return
	}
	// First, read the current Open and Close times for the event from the DB.
	// This also validates that the event ID is correct.
	// We save these for later to restore state after we're done.
	row, err := c.Database.LoadEvent(EventID)
	if err != nil {
		slog.Error(fmt.Sprintf("loading event #1: %v", err))
		return
	}
	var prevOpen time.Time
	var prevClose time.Time
	var prevDetails string
	for row.Next() {
		var eid string
		var open string
		var close string
		var details string // unused, no state to store.
		if err := row.Scan(&eid, &open, &close, &details); err != nil {
			slog.Error(fmt.Sprintf("could not scane item event row: %v", err))
			continue
		}
		if eid != EventID {
			slog.Error("the given event id was not found in the database")
			return
		}
		prevOpen, err = time.Parse(time.DateTime, open)
		if err != nil {
			slog.Error(fmt.Sprintf("parse open: %v", err))
			return
		}
		prevClose, err = time.Parse(time.DateTime, close)
		prevDetails = details
		slog.Info(fmt.Sprintf("previous values: open %s; close %s; details %s", open, close, details))
	}

	// Next, write dummy values into the DB.  This ensures that the item event
	// gets created in the CLOSED state.  Then we can .Open(), .Close(), and
	// .Resolve() so it gets the correct bets to redo the bet.
	tx, err := c.Database.OpenTransaction()
	if err != nil {
		slog.Error(fmt.Sprintf("open tx #1: %v", err))
		return
	}
	if err := tx.WriteOpened(EventID, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		slog.Error(fmt.Sprintf("write opened: %v", err))
		return
	}
	if err := tx.WriteClosed(EventID, time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)); err != nil {
		slog.Error(fmt.Sprintf("write closed: %v", err))
		return
	}

	// Next, refund given the config's refund map.
	for uid, amount := range environment.Event.BadEarnings {
		user, err := c.GetUser(uid)
		if err != nil {
			slog.Error(fmt.Sprintf("get user %s: %v", uid, err))
			return
		}
		if err := user.Earn(tx, -amount); err != nil {
			slog.Error(fmt.Sprintf("user %s earn: %v", uid, err))
			return
		}
		c.SendMessage(environment.DiscordChannel, fmt.Sprintf("Adjusted <@%s> balance %+d", uid, -amount))
	}
	// Point of no return!
	if err := tx.Commit(); err != nil {
		slog.Error(fmt.Sprintf("tx commit #1: %v", err))
		return
	}
	slog.Info("Database modified.  Failures after this point will require manual resolution.")

	// Create the event, and redo the event
	event := events.NewItemEvent(c, env.ItemEventConfig{ID: EventID}, environment.DiscordChannel)
	if err := event.Open(OpenTS); err != nil {
		slog.Error(fmt.Sprintf("on open: %v", err))
		return
	}
	event.Update(environment.Event.Actual)
	if err := event.Close(CloseTS); err != nil {
		slog.Error(fmt.Sprintf("on close: %v", err))
		return
	}
	if err := event.Resolve(); err != nil {
		slog.Error(fmt.Sprintf("on resolve: %v", err))
		return
	}

	// Finally, restore the old event details so we can go straight back to the
	// main betting binary.
	tx, err = c.Database.OpenTransaction()
	if err != nil {
		slog.Error(fmt.Sprintf("open tx #2: %v", err))
		return
	}
	if err := tx.WriteOpened(EventID, prevOpen); err != nil {
		slog.Error(fmt.Sprintf("write prev opened: %v", err))
		return
	}
	if err := tx.WriteClosed(EventID, prevClose); err != nil {
		slog.Error(fmt.Sprintf("write prev closed: %v", err))
		return
	}
	if err := tx.WriteEventDetails(EventID, prevDetails); err != nil {
		slog.Error(fmt.Sprintf("write prev details: %v", err))
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Error(fmt.Sprintf("tx commit #2: %v", err))
		return
	}
}
