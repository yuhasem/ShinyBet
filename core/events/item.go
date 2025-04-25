package events

import (
	"bet/core"
	"bet/core/db"
	"bet/state"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"sync"
	"time"
)

const itemEventName = "item"

type ItemEvent struct {
	// The species and held item to watch for.  Together, the event ends when
	// the first shiny of `species` is found, and the resolution of the event
	// depends on whether or not the species holds the `item`.
	species string
	item    string
	// State keeping for lifecycle
	mu         sync.Mutex
	state      EventState
	resolution bool
	c          *core.Core
}

func (e *ItemEvent) Notify(s *state.State) {
	if s.Encounter.IsShiny && equalIgnoreCase(s.Encounter.Species.Name, e.species) {
		// First update
		e.Update(equalIgnoreCase(s.Encounter.HeldItem.Name, e.item))
		// The event is closing.
		if err := e.Close(time.Now()); err != nil {
			slog.Error(fmt.Sprintf("error closing item event: %v", err))
			return
		}
		if err := e.Resolve(); err != nil {
			slog.Error(fmt.Sprintf("error resolving item event: %v", err))
			return
		}
		// The event is intentionally not reopened.
	}
}

func equalIgnoreCase(a, b string) bool {
	return strings.ToLower(a) == strings.ToLower(b)
}

func (e *ItemEvent) Open(t time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var err error
	e.state, err = commonOpen(e.c.Database, itemEventName, t, e.state)
	if err != nil {
		return err
	}
	return nil
}

// Oh shit, I want a different interface here.  Is it time to change to any?
func (e *ItemEvent) Update(value any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	v := value.(bool)
	e.resolution = v
}

func (e *ItemEvent) Close(t time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var err error
	e.state, err = commonClose(e.c.Database, itemEventName, t, e.state)
	return err
	return nil
}

func (e *ItemEvent) Resolve() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.c.EventMu.Lock()
	defer e.c.EventMu.Unlock()

	tx, err := e.c.Database.OpenTransaction()
	if err != nil {
		return err
	}
	bets, err := e.loadItemBets()
	if err != nil {
		return err
	}

	payout, winners, userContribution := e.calcPayout(bets)
	refund := false
	if winners == 0 {
		refund = true
	}

	e.resolveBets(tx, bets, refund)
	e.payoutWinners(tx, payout, winners, userContribution)
	// TODO: send message

	if err := tx.Commit(); err != nil {
		return err
	}
	e.state = CLOSED
	return nil
}

type itemBet struct {
	uid    string
	amount int
	guess  bool
}

func (e *ItemEvent) loadItemBets() ([]itemBet, error) {
	rows, err := e.c.Database.LoadBets(itemEventName)
	if err != nil {
		return nil, err
	}
	b := make([]itemBet, 0)
	for rows.Next() {
		var uid string
		var eid string    // unused
		var placed string // unused
		var amount int
		var risk float64 // unused
		var bet string   // TODO: verify this works?  It doesn't with the fake db.
		if err := rows.Scan(&uid, &eid, &placed, &amount, &risk, &bet); err != nil {
			continue
		}
		guess := false
		if bet == "true" {
			guess = true
		}
		b = append(b, itemBet{uid: uid, amount: amount, guess: guess})
	}
	return b, nil
}

func (e *ItemEvent) calcPayout(bets []itemBet) (int, int, map[string]int) {
	var payout int
	var winners int
	userContribution := make(map[string]int)
	for _, b := range bets {
		if b.guess != e.resolution {
			payout += b.amount
		} else {
			winners += b.amount
			userContribution[b.uid] += b.amount
		}
	}
	return payout, winners, userContribution
}

func (e *ItemEvent) resolveBets(tx db.Transaction, bets []itemBet, refund bool) {
	for _, b := range bets {
		loss := true
		if b.guess == e.resolution {
			loss = false
		}
		if refund {
			loss = false
		}
		u, err := e.c.GetUser(b.uid)
		if err != nil {
			continue
		}
		if err := u.Resolve(tx, b.amount, loss); err != nil {
			continue
		}
	}
}

func (e *ItemEvent) payoutWinners(tx db.Transaction, payout int, winners int, userContribution map[string]int) {
	ratio := float64(payout) / float64(winners)
	for uid, contribution := range userContribution {
		u, err := e.c.GetUser(uid)
		if err != nil {
			continue
		}
		gain := int(math.Ceil(float64(contribution) * ratio))
		if err := u.Earn(tx, gain); err != nil {
			continue
		}
	}
}
