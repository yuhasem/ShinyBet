package core

import (
	"bet/core/db"
	"fmt"
	"log"
	"sync"
)

type Core struct {
	// users is a map from id to user object.  Loading users from the database
	// doesn't really work inside transactions (get a lot of "database is
	// locked" errors) so this caching prevents that.  The downside is that this
	// requires diligence for all updates here to be reflected in the database
	// and vice versa.
	users map[string]*user
	// eventMu is a mutex to ensure that event closures do not overwrite user's
	// state when committing to storage.
	eventMu sync.Mutex
	// Events is the list of events that Core is handling
	events map[string]Event
	// Database is used for persisting new users.
	Database db.Database
}

func New(d db.Database) *Core {
	rows, err := d.LoadUsers()
	if err != nil {
		log.Printf("error loading users: %v", err)
		return nil
	}
	users := make(map[string]*user)
	for rows.Next() {
		u, err := loadUser(rows)
		if err != nil {
			log.Printf("error loading user: %v", err)
			continue
		}
		users[u.id] = u
	}
	return &Core{
		users:    users,
		events:   make(map[string]Event),
		Database: d,
	}
}

// //////////////////
// User Operations //
// //////////////////
func (c *Core) GetUser(id string) (*user, error) {
	// rows, err := c.Database.LoadUser(id)
	// if err != nil {
	// 	return nil, err
	// }
	// loaded := 0
	// for rows.Next() {
	// 	loaded++
	// 	u, err := loadUser(rows)
	// 	if u != nil {
	// 		return u, nil
	// 	}
	// 	log.Printf("error loading user: %v", err)
	// }
	// // No rows, so assume this is a new user interacting for the first time.
	// if loaded == 0 {
	// 	t, err := c.Database.OpenTransaction()
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	u, err := newUser(id, t)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	if err := t.Commit(); err != nil {
	// 		return nil, err
	// 	}
	// 	return u, nil
	// }
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
		return &EventAlreadyExistsErr{eventId: id}
	}
	c.events[id] = event
	return nil
}

func (c *Core) GetEvent(id string) (Event, error) {
	if e, ok := c.events[id]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("event of id %s is not registered", id)
}
