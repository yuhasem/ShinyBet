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
	// A name to display to users
	displayName string
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

func (p *phaseLifecycle) Update(value any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state != OPEN {
		return
	}
	phase := value.(int)
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
	// Grab the event mutex so no more bets or updates can come in.
	p.mu.Lock()
	defer p.mu.Unlock()
	// Grab core's event lock as well, so we don't stomp on other events making
	// user changes.
	p.core.EventMu.Lock()
	defer p.core.EventMu.Unlock()
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

	message := fmt.Sprintf("%s event closed! Phase was %d", p.displayName, p.current)

	payout, winnerTotal, userContribution := calculatePayout(bets, p.current)
	refundAll := false
	if winnerTotal == 0.0 {
		slog.Info(fmt.Sprintf("Nobody won the %s event", p.eventId))
		message += "\nNo winning bets!  No changes to user balances."
		refundAll = true
	}
	userDelta := resolveBets(p.core, tx, bets, p.current, refundAll)
	slog.Debug(fmt.Sprintf("userDelta after resolveBets: %+v", userDelta))
	if winnerTotal != 0.0 {
		userDelta = distributePayout(p.core, tx, payout, winnerTotal, userContribution, userDelta)
	}
	slog.Debug(fmt.Sprintf("userDelta after distributePayout: %+v", userDelta))
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
	p.state = CLOSED
	return nil
}

func loadPhaseBets(d db.Database, eid string) ([]*internalPhaseBet, error) {
	rows, err := d.LoadBets(eid)
	if err != nil {
		return nil, fmt.Errorf("could not load %s bets: %v", eid, err)
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
	for i, d := range deltas {
		user, err := c.GetUser(d.uid)
		if err != nil {
			slog.Warn(fmt.Sprintf("error getting user while making message: %s", err))
			continue
		}
		balance, _, err := user.Balance()
		if err != nil {
			slog.Warn(fmt.Sprintf("error getting users balance: %s", err))
		}
		nextDelta := fmt.Sprintf("\n * <@%s>: %+d (new balance %d cakes)", d.uid, d.amount, balance)
		if len(message)+len(nextDelta) >= 1975 {
			// The message will be too long for Discord, so cut it off here and
			// add a little suffix.
			// TODO: Could we print multiple messages instead?  This is the one
			// place I wouldn't want to compromise.
			message += fmt.Sprintf("and %d other participants", len(deltas)-i)
			break
		}
		message += nextDelta
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
	// Lock to check p.state, and for p.risk which needs a consistent view of
	// p.current
	p.mu.Lock()
	defer p.mu.Unlock()
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
	if bet.Phase <= p.current {
		return 0.0, PhaseLengthError{}
	}
	length := float64(bet.Phase - p.current)
	inverseProb := 1.0 - p.probability
	if bet.Direction == LESS {
		// risk(<x) = P(>=x) = P(>x-1) = (1-p)^(x-1)
		return math.Pow(inverseProb, length-1.0), nil
	}
	if bet.Direction == GREATER {
		// risk(>x) = 1 - P(>x) = 1 - (1-p)^x
		return 1 - math.Pow(inverseProb, length), nil
	}
	if bet.Direction == EQUAL {
		// risk(=x) = 1 - P(=x) = 1 - p*(1-p)^(x-1)
		return 1.0 - p.probability*math.Pow(inverseProb, length-1.0), nil
	}
	return 0.0, fmt.Errorf("unknown direction %d", bet.Direction)
}

func (p *phaseLifecycle) Interpret(blob string) string {
	b := phaseBetFrom(blob)
	return interpretPhaseBet(b)
}

func (p *phaseLifecycle) BetsSummary(style string) (string, error) {
	bets, err := loadPhaseBets(p.core.Database, p.eventId)
	if err != nil {
		return "", err
	}
	// First collect cumulative stats on the bets.  Unresolved bets and the
	// amount in them, guaranteed losses, guarantted winners (risk adjusted),
	// and total balance on the event.
	var inUnresolved int
	unresolvedBets := make([]*internalPhaseBet, 0, len(bets))
	var inLosers int
	var inWinners float64
	var total int
	for _, b := range bets {
		total += b.amount
		if b.bet.Phase >= p.current {
			unresolvedBets = append(unresolvedBets, b)
			inUnresolved += b.amount
			continue
		}
		if b.bet.Direction == GREATER {
			inWinners += float64(b.amount) * b.risk
		} else {
			inLosers += b.amount
		}
	}
	message := fmt.Sprintf("There are %d cakes in bets on the %s event. (Current phase: %d)\n", total, p.displayName, p.current)
	message += fmt.Sprintf(" * %d cakes are guaranteed to be in the payout\n", inLosers)
	message += fmt.Sprintf(" * %d cakes are in unresolved bets\n", inUnresolved)
	message += fmt.Sprintf(" * %.2f is the risk adjusted pool of guaranteed winners\n", inWinners)

	// Next, sort the unresolved bets, so we can display the most important.
	switch style {
	case "risk":
		slices.SortFunc(unresolvedBets, sortByAdjustedRisk)
		message += "\nThe unresolved bets with the highest risk adjusted factor are:"
	case "soon":
		slices.SortFunc(unresolvedBets, sortByUpcoming)
		message += "\nThe unresolved bets that will resolve next are:"
	}
	for i, b := range unresolvedBets {
		// Discord has a limit of 2000 characters per message.  This limit gives
		// us some leeway for the last append being oversized, and still enough
		// room to write a closing message.
		if len(message) > 1880 {
			message += fmt.Sprintf("\nAnd %d other bets.", len(unresolvedBets)-i)
			return message, nil
		}
		message += fmt.Sprintf("\n * <@%s> placed %d cakes on %s (%.2f%% risk)", b.uid, b.amount, interpretPhaseBet(b.bet), b.risk*100)
	}
	return message, nil
}

func sortByAdjustedRisk(a, b *internalPhaseBet) int {
	diff := float64(b.amount)*b.risk - float64(a.amount)*a.risk
	if diff > 0 {
		return 1
	} else if diff < 0 {
		return -1
	}
	return 0
}

func sortByUpcoming(a, b *internalPhaseBet) int {
	return a.bet.Phase - b.bet.Phase
}
