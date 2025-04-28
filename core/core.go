package core

import (
	"bet/core/db"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type Core struct {
	// users is a map from id to user object.  Loading users from the database
	// doesn't really work inside transactions (get a lot of "database is
	// locked" errors) so this caching prevents that.  The downside is that this
	// requires diligence for all updates here to be reflected in the database
	// and vice versa.
	users map[string]*user
	// eventMu is a mutex to ensure that event closures do not overwrite user's
	// state when committing to storage.
	EventMu sync.Mutex
	// Events is the list of events that Core is handling
	events map[string]Event
	// Database is used for persisting new users.
	Database db.Database
	// session is the Discord session that can be used for interacting outside
	// of commands
	session InteractionSession
	// clock is used for time controls in crons. It is injected so that it can
	// be used in unit tests.
	clock Clock
}

func New(d db.Database, session InteractionSession, clock Clock) *Core {
	rows, err := d.LoadUsers()
	if err != nil {
		slog.Error(fmt.Sprintf("error loading users: %v", err))
		return nil
	}
	users := make(map[string]*user)
	for rows.Next() {
		u, err := loadUser(rows)
		if err != nil {
			slog.Warn(fmt.Sprintf("error loading user: %v", err))
			continue
		}
		users[u.id] = u
	}
	return &Core{
		users:    users,
		events:   make(map[string]Event),
		Database: d,
		session:  session,
		clock:    clock,
	}
}

func (c *Core) Close() {}

// //////////////////
// User Operations //
// //////////////////
func (c *Core) GetUser(id string) (*user, error) {
	u, ok := c.users[id]
	if ok {
		return u, nil
	}
	// New user interacting.
	t, err := c.Database.OpenTransaction()
	if err != nil {
		return nil, err
	}
	u, err = newUser(id, t)
	if err != nil {
		return nil, err
	}
	if err := t.Commit(); err != nil {
		return nil, err
	}
	c.users[id] = u
	return u, nil
}

// RefreshBalance makes it so that all users have at least 100 balance
// remaining.
func (c *Core) RefreshBalance() error {
	slog.Debug("RefreshBalance called")
	tx, err := c.Database.OpenTransaction()
	if err != nil {
		return err
	}
	if err := tx.RefreshBalance(); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// This is SUPER ugly, but because of transaction/locking, it's kind of a
	// necessary evil for now.
	for _, u := range c.users {
		if u.balance < 100 {
			u.balance = 100
		}
	}
	return nil
}

// ///////////////////
// Event Operations //
// ///////////////////
type EventAlreadyExistsErr struct {
	eventId string
}

func (e *EventAlreadyExistsErr) Error() string {
	return fmt.Sprintf("an event with id %q already exists", e.eventId)
}

// RegisterEvent registers the given event with the given id, so that bet
// placers can get the event outside of load time.  It is an error to register
// 2 events by the same id.
func (c *Core) RegisterEvent(id string, event Event) error {
	if _, ok := c.events[id]; ok {
		slog.Warn(fmt.Sprintf("duplicate event registration: %s", id))
		return &EventAlreadyExistsErr{eventId: id}
	}
	c.events[id] = event
	return nil
}

func (c *Core) GetEvent(id string) (Event, error) {
	if e, ok := c.events[id]; ok {
		return e, nil
	}
	slog.Warn(fmt.Sprintf("request for non existent event: %s", id))
	return nil, fmt.Errorf("event of id %s is not registered", id)
}

// ///////////////////////
// Discord Interactions //
// ///////////////////////
type InteractionSession interface {
	ChannelMessageSendComplex(string, *discordgo.MessageSend, ...discordgo.RequestOption) (*discordgo.Message, error)
}

func (c *Core) SendMessage(channel, message string) error {
	_, err := c.session.ChannelMessageSendComplex(channel, &discordgo.MessageSend{
		Content: message,
		AllowedMentions: &discordgo.MessageAllowedMentions{
			// By default, we don't allow mentions so there's so the bot doesn't
			// ping people awake.
			Parse: []discordgo.AllowedMentionType{},
		},
	})
	return err
}

// //////////////////
// Cron Operations //
// //////////////////
func (c *Core) AddCron(cron Cron) error {
	// TODO: attempt to load last run time from db.
	lastRun := c.clock.Now()
	go schedule(cron, c.clock, lastRun.Add(cron.After()))
	return nil
}

func schedule(cron Cron, clock Clock, at time.Time) {
	if !at.Before(clock.Now()) {
		wait := at.Sub(clock.Now())
		<-clock.After(wait)
	}
	for {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("recovering from panic in %q cron: %v", cron.ID(), r)
				}
			}()
			if err := cron.Run(); err != nil {
				slog.Error("error in %q cron: %v", cron.ID(), err)
			}
		}()
		<-clock.After(cron.After())
	}
}
