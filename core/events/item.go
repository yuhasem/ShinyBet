package events

import (
	"bet/core"
	"bet/core/db"
	"bet/env"
	"bet/state"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strings"
	"sync"
	"time"
)

const itemEventName = "item"

type ItemEvent struct {
	// The species and held item to watch for.  Together, the event ends when
	// the first shiny of `species` is found, and the resolution of the event
	// depends on whether or not the species holds the `item`.
	species  string
	item     string
	ID       string
	prob     float64
	keepOpen bool
	cond     env.Condition
	// State keeping for lifecycle
	mu         sync.Mutex
	state      EventState
	resolution bool
	c          *core.Core
	channel    string
}

func NewItemEvent(c *core.Core, conf env.ItemEventConfig, channel string) *ItemEvent {
	id := conf.ID
	if id == "" {
		id = itemEventName
	}
	e := &ItemEvent{
		c:        c,
		species:  conf.Species,
		item:     conf.Item,
		ID:       id,
		prob:     conf.Probability,
		keepOpen: conf.KeepOpen,
		cond:     conf.KeepOpenCondition,
		channel:  channel,
	}
	e = loadItemEvent(e)
	return e
}

// Modifies ItemEvent in place, if there the database has information about this
// event, writing a new row to the databse if none is present.  Errors will
// result in the event being returned unchanged
func loadItemEvent(e *ItemEvent) *ItemEvent {
	row, err := e.c.Database.LoadEvent(e.ID)
	if err != nil {
		slog.Error("could not load item event from db: %v", err)
		return e
	}
	gotRow := false
	for row.Next() {
		gotRow = true
		var eid string // unused
		var open string
		var close string
		var details string // unused, no state to store.
		if err := row.Scan(&eid, &open, &close, &details); err != nil {
			slog.Error(fmt.Sprintf("could not scane item event row: %v", err))
			continue
		}
		openTs, err := time.Parse(time.DateTime, open)
		if err != nil {
			slog.Error(fmt.Sprintf("could not parse open time from db: %v", err))
			continue
		}
		closeTs, err := time.Parse(time.DateTime, close)
		if err != nil {
			slog.Error(fmt.Sprintf("could not parse close time from db: %v", err))
			continue
		}
		if !closeTs.After(openTs) {
			e.state = OPEN
		}
	}
	if gotRow {
		return e
	}
	tx, err := e.c.Database.OpenTransaction()
	if err != nil {
		slog.Error(fmt.Sprintf("could not open transaction to write new item row"))
		return e
	}
	if err := tx.WriteNewEvent(e.ID, time.Now(), ""); err != nil {
		slog.Error(fmt.Sprintf("could not write item event row: %v", err))
		return e
	}
	if err := tx.Commit(); err != nil {
		slog.Error(fmt.Sprintf("could not commit item event row: %v", err))
		return e
	}
	return e
}

func (e *ItemEvent) Notify(s *state.State) {
	slog.Debug("start item notify")
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
		if e.keepOpen {
			if ShouldKeepOpen(s, e.species, e.cond) {
				if err := e.Open(time.Now()); err != nil {
					slog.Error(fmt.Sprintf("error opening item event: %v", err))
					return
				}
			}
		}
	}
	slog.Debug("end item notify")
}

func ShouldKeepOpen(s *state.State, species string, cond env.Condition) bool {
	if cond == (env.Condition{}) {
		return true
	}
	p, ok := s.Stats.Pokemon[species]
	if !ok {
		// We've never seen the mon?  Shouldn't be true if we just saw a shiny
		// for it.  Close the event, just in case.
		slog.Warn("item event closed but didn't have any of that species in stats")
		return false
	}
	if p.ShinyEncounters < cond.ShiniesLessThan {
		return true
	}
	return false
}

func equalIgnoreCase(a, b string) bool {
	return strings.ToLower(a) == strings.ToLower(b)
}

func (e *ItemEvent) Open(t time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var err error
	e.state, err = commonOpen(e.c.Database, e.ID, t, e.state)
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
	e.state, err = commonClose(e.c.Database, e.ID, t, e.state)
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

	userDelta := e.resolveBets(tx, bets, refund)
	userDelta = e.payoutWinners(tx, payout, winners, userContribution, userDelta)
	if e.channel != "" {
		e.sendMessage(userDelta)
	}

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
	rows, err := e.c.Database.LoadBets(e.ID)
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

func (e *ItemEvent) resolveBets(tx db.Transaction, bets []itemBet, refund bool) map[string]int {
	userDelta := make(map[string]int)
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
		if loss {
			userDelta[b.uid] -= b.amount
		}
	}
	return userDelta
}

func (e *ItemEvent) payoutWinners(tx db.Transaction, payout int, winners int, userContribution map[string]int, userDelta map[string]int) map[string]int {
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
		userDelta[uid] += gain
	}
	return userDelta
}

func (e *ItemEvent) sendMessage(userDelta map[string]int) {
	message := strings.Builder{}
	dir := "was NOT holding"
	if e.resolution {
		dir = "was HOLDING"
	}
	fmt.Fprintf(&message, "Held Item event ended! %s %s %s\n", e.species, dir, e.item)

	deltas := sortedDeltas(userDelta)
	if len(deltas) < 0 {
		fmt.Fprintf(&message, "No changes to user balances.")
	} else {
		fmt.Fprintf(&message, "\nNet cake gain/loss:")
	}
	for i, d := range deltas {
		user, err := e.c.GetUser(d.uid)
		if err != nil {
			slog.Warn(fmt.Sprintf("error getting user while making message: %s", err))
			continue
		}
		balance, _, err := user.Balance()
		if err != nil {
			slog.Warn(fmt.Sprintf("error getting users balance: %s", err))
		}
		nextDelta := fmt.Sprintf("\n * %s (new balance %d cakes)", d, balance)
		if len(nextDelta)+message.Len() >= 1975 {
			fmt.Fprintf(&message, "and %d other participants", len(deltas)-i)
			break
		}
		message.WriteString(nextDelta)
	}
	if err := e.c.SendMessage(e.channel, message.String()); err != nil {
		slog.Warn(fmt.Sprintf("error sending closing message: %v", err))
	}
}

func (e *ItemEvent) Wager(uid string, amount int, placed time.Time, bet any) (any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	wagerReqs.WithLabelValues(e.ID).Inc()
	if e.state != OPEN {
		return 0.0, BettingClosedError{}
	}
	guess, ok := bet.(bool)
	if !ok {
		return 0.0, fmt.Errorf("bet argument must be of type bool")
	}
	risk := e.prob
	if guess {
		risk = 1 - e.prob
	}
	user, err := e.c.GetUser(uid)
	if err != nil {
		return 0.0, err
	}
	tx, err := e.c.Database.OpenTransaction()
	if err != nil {
		return 0.0, err
	}
	if err := user.Reserve(tx, amount); err != nil {
		return 0.0, err
	}
	if err := tx.WriteBet(uid, e.ID, placed, amount, risk, fmt.Sprintf("%t", guess)); err != nil {
		return 0.0, err
	}
	if err := tx.Commit(); err != nil {
		return 0.0, err
	}
	wagerSuccess.WithLabelValues(e.ID).Inc()
	return risk, nil
}

func (e *ItemEvent) Interpret(blob string) string {
	if blob == "true" {
		return fmt.Sprintf("%s WILL hold %s", e.species, e.item)
	}
	return fmt.Sprintf("%s will NOT hold %s", e.species, e.item)
}

func (e *ItemEvent) BetsSummary(style string) (string, error) {
	if e.state != OPEN {
		return "The Held Item event is closed.", nil
	}
	bets, err := e.loadItemBets()
	if err != nil {
		return "", err
	}
	// No sorting for soon, becuase that doeesn't really make sense in this
	// context.
	switch style {
	case "risk":
		slices.SortFunc(bets, func(a, b itemBet) int {
			return b.amount - a.amount
		})
	}
	// we can build the bottom part of the message at the same time we're
	// collecting totals because bets don't become resolved like they do for
	// phase bets
	betMessage := strings.Builder{}
	var total int
	var neg int
	var pos int
	for _, b := range bets {
		total += b.amount
		var m string
		if b.guess {
			pos += b.amount
			m = "WILL hold"
		} else {
			neg += b.amount
			m = "will NOT hold"
		}
		fmt.Fprintf(&betMessage, " * <@%s> placed %d cakes that %s %s %s\n", b.uid, b.amount, e.species, m, e.item)
	}
	negFrac := float64(neg) / float64(total)
	posFrac := float64(pos) / float64(total)
	message := fmt.Sprintf("There are %d cakes on the Item event\n", total)
	message += fmt.Sprintf("%d (%.2f%%) AGAINST : FOR %d (%.2f%%)\n", neg, 100*negFrac, pos, 100*posFrac)
	message = fmt.Sprintf("%s%s", message, betMessage.String())
	return message, nil
}
