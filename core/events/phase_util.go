// phase_util contains common definitions for events which rely on a stream of
// events until one happens with a specific probability.  This is primarily
// focused on serving the "shiny" and "antishiny" events.
package events

import (
	"bet/core"
	"bet/core/db"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	currentPhase = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "core_events_shiny_current_phase",
		Help: "The current phase seen by phase events",
	},
		[]string{
			// The event that sees this phase
			"event",
		})
	wagerReqs = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "core_events_shiny_wagers_total",
		Help: "Total number of times Wager was called for shiny event.",
	},
		[]string{
			// The event that was wagered on
			"event",
		})
	wagerSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "core_events_shiny_wagers_success",
		Help: "Total number of times Wager call succeeded for shiny event.",
	},
		[]string{
			// The event that was wagered on
			"event",
		})
)

// State enum
const (
	CLOSED = iota
	OPEN
	CLOSING
)

// Direction enum
const (
	LESS = iota
	EQUAL
	GREATER
)

type PhaseBet struct {
	Direction int
	Phase     int
}

// PlacedPhaseBet is the return from Wager(), that can be used to send a
// detailed message to the user about the bet placed.
type PlacedPhaseBet struct {
	Amount int
	Risk   float64
}

// Creates a bet from a string loaded from storage.
func phaseBetFrom(str string) PhaseBet {
	var ret PhaseBet
	parts := strings.Split(str, ",")
	if len(parts) < 2 {
		// oh no.
		return ret
	}
	dir, err := strconv.Atoi(parts[0])
	if err != nil || dir < 0 || dir > 2 {
		return ret
	}
	ret.Direction = dir
	phase, err := strconv.Atoi(parts[1])
	if err != nil {
		return ret
	}
	ret.Phase = phase
	return ret
}

// Creates a string suitable for storing this bet.
func (b PhaseBet) storage() string {
	return fmt.Sprintf("%d,%d", b.Direction, b.Phase)
}

func interpretPhaseBet(bet PhaseBet) string {
	sign := ""
	switch bet.Direction {
	case LESS:
		sign = "<"
	case GREATER:
		sign = ">"
	case EQUAL:
		sign = "="
	}
	return fmt.Sprintf("phase %s %d", sign, bet.Phase)
}

type internalPhaseBet struct {
	amount int
	bet    PhaseBet
	risk   float64
	uid    string
}

type StateMachineError struct {
	expected int
	actual   int
}

func (err StateMachineError) Error() string {
	return fmt.Sprintf("wrong state for transition, expected %d, was %d", err.expected, err.actual)
}

// phaseLifecycle implements lifecycle management methods (Open, Update, and
// Close) and the command method Wager, to be used in composing phase events.
type phaseLifecycle struct {
	// The eventId used to read from and write to the database.
	eventId string
	// The probability of the betting event occurring at each encounter.
	probability float64
	// A reference to the Core to use for user and database commands.
	core *core.Core
	// The Discord channel to send a message in
	channel string

	// The following don't need to be initialized.
	mu sync.Mutex
	// see State enum
	state   int
	current int
	// opened  time.Time
}

// Open updates the database for the open time and resets state for tracking the
// phase.  It is safe to call Open while the event is already open.
func (p *phaseLifecycle) Open(open time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != CLOSED {
		return StateMachineError{expected: CLOSED, actual: p.state}
	}

	tx, err := p.core.Database.OpenTransaction()
	if err != nil {
		return err
	}
	if err := tx.WriteOpened(p.eventId, open); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.state = OPEN
	p.current = 0
	return nil
}

func (p *phaseLifecycle) Update(phase int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != OPEN {
		return
	}
	p.current = phase
	currentPhase.WithLabelValues(p.eventId).Set(float64(phase))
}

func (p *phaseLifecycle) Close(close time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != OPEN {
		return StateMachineError{expected: OPEN, actual: p.state}
	}

	tx, err := p.core.Database.OpenTransaction()
	if err != nil {
		return err
	}
	if err := tx.WriteClosed(p.eventId, close); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	p.state = CLOSING
	return nil
}

// Resolve resolves all wagers between last open and last close and sets the
// event to the CLOSED state.  This is not part of the Event interface, but
// allows for better timing control between CLOSING and CLOSED.
func (p *phaseLifecycle) Resolve() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != CLOSING {
		return StateMachineError{expected: CLOSING, actual: p.state}
	}

	bets, err := loadPhaseBets(p.core.Database, p.eventId)
	if err != nil {
		return err
	}
	tx, err := p.core.Database.OpenTransaction()
	if err != nil {
		return err
	}

	message := fmt.Sprintf("Shiny event closed! Phase was %d", p.current)

	payout, winnerTotal, userContribution := calculatePayout(bets, p.current)
	refundAll := false
	if winnerTotal == 0.0 {
		slog.Info(fmt.Sprintf("Nobody won the %d event", p.eventId))
		message += "\nNo winning bets!  No changes to user balances."
		refundAll = true
	}
	userDelta := resolveBets(p.core, tx, bets, p.current, refundAll)
	if winnerTotal != 0.0 {
		userDelta = distributePayout(p.core, tx, payout, winnerTotal, userContribution, userDelta)
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// In a separate transaction, refresh balances of people who got too low, so
	// they can continue to play.
	if err := p.core.RefreshBalance(); err != nil {
		return err
	}

	if p.channel != "" {
		sendMessage(p.core, p.channel, message, userDelta)
	}
	return nil
}

func loadPhaseBets(d db.Database, eid string) ([]*internalPhaseBet, error) {
	rows, err := d.LoadBets(eid)
	if err != nil {
		return nil, fmt.Errorf("could not load shiny bets: %v", err)
	}
	bs := make([]*internalPhaseBet, 0)
	for rows.Next() {
		var uid string
		var eid string
		// ignored
		var placed string
		var amount int
		var risk float64
		var bet string
		if err := rows.Scan(&uid, &eid, &placed, &amount, &risk, &bet); err != nil {
			slog.Warn(fmt.Sprintf("unable to scan bet row: %s", err))
			continue
		}
		bs = append(bs, &internalPhaseBet{
			amount: amount,
			bet:    phaseBetFrom(bet),
			risk:   risk,
			uid:    uid,
		})

	}
	return bs, nil
}

// Returns the payout, the winner's total weight, and a map from user to weight
// contributed to the winner's total weight.
func calculatePayout(bets []*internalPhaseBet, phase int) (int, float64, map[string]float64) {
	var payout int
	var winnerTotal float64
	userContribution := make(map[string]float64)
	for _, b := range bets {
		switch b.bet.Direction {
		case LESS:
			if phase < b.bet.Phase {
				contribution := float64(b.amount) * b.risk
				userContribution[b.uid] += contribution
				winnerTotal += contribution
			} else {
				payout += b.amount
			}
		case GREATER:
			if phase > b.bet.Phase {
				contribution := float64(b.amount) * b.risk
				userContribution[b.uid] += contribution
				winnerTotal += contribution
			} else {
				payout += b.amount
			}
		case EQUAL:
			if phase == b.bet.Phase {
				contribution := float64(b.amount) * b.risk
				userContribution[b.uid] += contribution
				winnerTotal += contribution
			} else {
				payout += b.amount
			}
		}
	}
	return payout, winnerTotal, userContribution
}

// Resolves the bets and returns a map of user ids to losses to be used in the
// output message creation.
func resolveBets(c *core.Core, tx db.Transaction, bets []*internalPhaseBet, phase int, refundAll bool) map[string]int {
	userDelta := make(map[string]int)
	for _, b := range bets {
		user, err := c.GetUser(b.uid)
		if err != nil {
			slog.Warn(fmt.Sprintf("Could not load user %s while resolving bets", b.uid))
			continue
		}
		loss := true
		if b.bet.Direction == LESS && phase < b.bet.Phase {
			loss = false
		} else if b.bet.Direction == GREATER && phase > b.bet.Phase {
			loss = false
		} else if b.bet.Direction == EQUAL && phase == b.bet.Phase {
			loss = false
		}
		if refundAll {
			loss = false
		}
		if loss {
			userDelta[b.uid] -= b.amount
		}
		if err := user.Resolve(tx, b.amount, loss); err != nil {
			slog.Warn(fmt.Sprintf("Could not resolve a users bet in Close(): %v", err))
			continue
		}
	}
	return userDelta
}

func distributePayout(c *core.Core, tx db.Transaction, payout int, winnerTotal float64, userContribution map[string]float64, userDelta map[string]int) map[string]int {
	fPayout := float64(payout)
	for uid, contribution := range userContribution {
		user, err := c.GetUser(uid)
		if err != nil {
			continue
		}
		if contribution == 0.0 {
			// Technically not needed, but prevents us from doing an extra
			// lock/unlock on the player's mutex, and potentially we want to
			// signal something back to the user when all of their bets were
			// dropped because the phase was too low.
			continue
		}
		amount := int(math.Ceil(fPayout * contribution / winnerTotal))
		userDelta[uid] += amount
		if err := user.Earn(tx, amount); err != nil {
			slog.Warn(fmt.Sprintf("error distributing payout: %s", err))
		}
	}
	return userDelta
}

func sendMessage(c *core.Core, channel string, message string, userDelta map[string]int) {
	// Collect user deltas into an array for sorting
	type delta struct {
		amount int
		uid    string
	}
	deltas := make([]delta, 0, len(userDelta))
	for uid, amount := range userDelta {
		deltas = append(deltas, delta{amount: amount, uid: uid})
	}
	// Sorts with highest delta first
	slices.SortFunc(deltas, func(a, b delta) int {
		return b.amount - a.amount
	})
	if len(deltas) > 0 {
		message += "\nNet cake gains/losses:"
	}
	for _, d := range deltas {
		user, _ := c.GetUser(d.uid)
		balance, _, err := user.Balance()
		slog.Warn(fmt.Sprintf("error getting users balance: %s", err))
		message += fmt.Sprintf("\n * <@%s>: %+d (new balance %d cakes)", d.uid, d.amount, balance)
	}
	if err := c.SendMessage(channel, message); err != nil {
		// don't make this error block anything
		slog.Warn(fmt.Sprintf("error sending message on shiny event close: %v", err))
	}
}

type NoRiskError struct{}

func (e NoRiskError) Error() string {
	return "risk was either 0 or 1, which is disallowed"
}

func (p *phaseLifecycle) Wager(uid string, amount int, placed time.Time, bet any) (any, error) {
	wagerReqs.WithLabelValues(p.eventId).Inc()
	if p.state != OPEN {
		return nil, fmt.Errorf("betting is closed")
	}
	b, ok := bet.(PhaseBet)
	if !ok {
		return nil, fmt.Errorf("fourth argument must be of type PhaseBet")
	}
	r, err := p.risk(b)
	if err != nil {
		return nil, err
	}
	if r == 0.0 || r == 1.0 {
		return nil, NoRiskError{}
	}
	user, err := p.core.GetUser(uid)
	if err != nil {
		return nil, err
	}
	transaction, err := p.core.Database.OpenTransaction()
	if err != nil {
		return nil, err
	}

	if err := user.Reserve(transaction, amount); err != nil {
		return nil, err
	}
	if err := transaction.WriteBet(uid, p.eventId, placed, amount, r, b.storage()); err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}
	wagerSuccess.WithLabelValues(p.eventId).Inc()
	return PlacedPhaseBet{Amount: amount, Risk: r}, nil
}

type PhaseLengthError struct {
}

func (p PhaseLengthError) Error() string {
	return "predicted phase cannot be less than the current phase"
}

func (p *phaseLifecycle) risk(bet PhaseBet) (float64, error) {
	// Lock to get a consistent view of current
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current < bet.Phase {
		return 0.0, PhaseLengthError{}
	}
	// prob is the probability of a phase lasting longer than the guess. That's
	// the risk for < bets.
	prob := math.Pow(p.probability, float64(bet.Phase-p.current))
	// For > bets, we need to reverse the odds to get risk
	if bet.Direction == GREATER {
		return 1 - prob, nil
	}
	if bet.Direction == EQUAL {
		// Effectively multiply by 8192/8191 to subtract 1 from the length used
		// to calculate psp, then multiply by 1/8192 for the shiny happening
		// exactly on the predicted phase.  That is the probability that it is
		// exactly that phase, so "1 -" to turn it into risk
		return 1.0 - (prob / 8191.0), nil
	}
	return prob, nil
}
