package events

import (
	"bet/core"
	"bet/state"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const shinyEventName = "shiny"

var (
	currentPhase = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "core/events/shiny_current_phase",
		Help: "The current phase as seen by the shiny event.",
	})
	wagerReqs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core/events/shiny_wagers_total",
		Help: "Total number of times Wager was called for shiny event.",
	})
	wagerSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core/events/shiny_wagers_success",
		Help: "Total number of times Wager call succeeded for shiny event.",
	})
)

type ShinyEvent struct {
	mu   sync.Mutex
	open bool
	// The length of the current phase, used to give odds to incoming bets.
	current int
	// The time the event was opened, used to keep track of which bets to use.
	opened time.Time
	core   *core.Core
	// Not persisted, but kept in memory to check an error condition.
	lastEncounterWasShiny bool
	// The discord session and channel is used to send a message when the event
	// closes.
	// TODO: This feels like something that should be abstracted similar to db.
	session *discordgo.Session
	channel string
}

func NewShinyEvent(c *core.Core, s *discordgo.Session, channel string) *ShinyEvent {
	e := &ShinyEvent{
		core:    c,
		session: s,
		channel: channel,
	}
	// Attempt to re-construct state by loading it from storage. Regardless of
	// the outcome, the event is the canonical one we want to register with
	// core.
	loadEvent(e)
	return e
}

func loadEvent(event *ShinyEvent) {
	rows, err := event.core.Database.LoadEvent("shiny")
	if err != nil {
		slog.Error(fmt.Sprintf("error querying data for load: %s", err))
		return
	}
	gotRow := false
	for rows.Next() {
		gotRow = true
		var eid string
		var lastOpen string
		var lastClose string
		var details []byte
		err := rows.Scan(&eid, &lastOpen, &lastClose, &details)
		if err != nil {
			slog.Error(fmt.Sprintf("error reading data for event row %s", err))
			return
		}
		openTs, err := time.Parse(time.DateTime, lastOpen)
		if err != nil {
			slog.Error(fmt.Sprintf("unable to parse last open time %s", err))
			return
		}
		closeTs, err := time.Parse(time.DateTime, lastClose)
		if err != nil {
			slog.Error(fmt.Sprintf("unable to parse last close time %s", err))
			return
		}
		// TODO: Update should write current phase length, and we should read it
		// back here.
		event.opened = openTs
		event.open = !closeTs.After(openTs)
	}
	if !gotRow {
		slog.Debug("no existing shiny event row")
		tx, err := event.core.Database.OpenTransaction()
		if err != nil {
			slog.Error(fmt.Sprintf("error opening transaction for new event: %v", err))
			return
		}
		if err := tx.WriteNewEvent(shinyEventName, time.Now(), ""); err != nil {
			slog.Error(fmt.Sprintf("error writing new event row: %v", err))
			return
		}
		if err := tx.Commit(); err != nil {
			slog.Error(fmt.Sprintf("error committing transaction for new event: %v", err))
			return
		}
		if err := event.Open(time.Now()); err != nil {
			slog.Error(fmt.Sprintf("could not open new event: %v", err))
		}
		return
	}
}

// Notify is satisfying the state.Observer interface.  This function is called
// when new state has been received and is our opportunity to update phase
// encounters and close the event if needed.
func (e *ShinyEvent) Notify(s *state.State) {
	if !e.lastEncounterWasShiny && s.Stats.CurrentPhase.Encounters < e.current {
		// TODO: this is the panic condition.  The phase has been reset and we
		// didn't see the encounter that caused it.  The goals are:
		// 1. Keep the bot running.  We can still move on and start tracking
		//    bets for the next phase before resolving the old one.
		// 2. Print as much debug info as we can so that a human can go in and
		//    verify/debug what has happened.
		// 3. Allow for manual close at a later time.
		slog.Warn("TODO: PANIC")
	}
	e.Update(s.Stats.CurrentPhase.Encounters)
	if s.Encounter.IsShiny {
		e.lastEncounterWasShiny = true
		slog.Debug(fmt.Sprintf("received state %+v", s))
		if err := e.Close(time.Now()); err != nil {
			slog.Error(fmt.Sprintf("error closing shiny event: %v", err))
		}
		// TODO: or should we open on the next encounter? i.e. because the catch
		// is happening.  I'd rather open up bets as early as possible to allow
		// for bets on the phase of 1.
		if err := e.Open(time.Now()); err != nil {
			slog.Error(fmt.Sprintf("error opening shiny event: %v", err))
		}
	} else {
		e.lastEncounterWasShiny = false
	}
}

type internalBet struct {
	amount int
	bet    Bet
	placed time.Time
	risk   float64
	uid    string
}

// PlacedBet is the return from Wager(), that can be used to send a detailed
// message to the user about the bet placed.
type PlacedBet struct {
	Amount int
	Risk   float64
}

const (
	LESS    = 0
	EQUAL   = 1
	GREATER = 2
)

// Bet is a bet that a shiny will be less than or greater than a phase length.
type Bet struct {
	Direction int
	Phase     int
}

// Creates a bet from a string loaded from storage.
func betFrom(str string) Bet {
	var ret Bet
	parts := strings.Split(str, ",")
	if len(parts) < 2 {
		// oh no.
		return ret
	}
	// TODO: the string matching code can be deleted once a new databse is being
	// used, since this is an old style before switching to enum based values.
	if parts[0] == "true" {
		ret.Direction = LESS
	} else if parts[0] == "false" {
		ret.Direction = GREATER
	} else {
		dir, err := strconv.Atoi(parts[0])
		if err != nil || dir < 0 || dir > 2 {
			return ret
		}
		ret.Direction = dir
	}
	phase, err := strconv.Atoi(parts[1])
	if err != nil {
		return ret
	}
	ret.Phase = phase
	return ret
}

// Creates a string suitable for storing this bet.
func (b Bet) storage() string {
	return fmt.Sprintf("%d,%d", b.Direction, b.Phase)
}

func (e *ShinyEvent) Open(t time.Time) error {
	if e.open {
		return nil
	}
	transaction, err := e.core.Database.OpenTransaction()
	if err != nil {
		return err
	}
	if err := transaction.WriteOpened(shinyEventName, t); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return err
	}
	e.open = true
	e.opened = t
	e.current = 0
	return nil
}

func (e *ShinyEvent) Update(current int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.current = current
	currentPhase.Set(float64(current))
}

func (e *ShinyEvent) Close(closed time.Time) error {
	if !e.open {
		return nil
	}
	bs, err := e.bets()
	if err != nil {
		return err
	}
	transaction, err := e.core.Database.OpenTransaction()
	if err != nil {
		return err
	}

	message := fmt.Sprintf("Shiny event closed! Phase was %d", e.current)
	userDelta := make(map[string]int)

	if err := transaction.WriteClosed("shiny", closed); err != nil {
		return err
	}
	e.open = false
	// Step 1: calculate payour, winner total, and per user contribution to the
	// winner total.
	payout := 0
	winnerTotal := 0.0
	userContributions := make(map[string]float64)
	for _, b := range bs {
		switch b.bet.Direction {
		case LESS:
			if e.current < b.bet.Phase {
				contribution := b.risk * float64(b.amount)
				userContributions[b.uid] += contribution
				winnerTotal += contribution
			} else {
				payout += b.amount
			}
		case GREATER:
			if e.current > b.bet.Phase {
				contribution := b.risk * float64(b.amount)
				userContributions[b.uid] += contribution
				winnerTotal += contribution
			} else {
				payout += b.amount
			}
		case EQUAL:
			if e.current == b.bet.Phase {
				contribution := b.risk * float64(b.amount)
				userContributions[b.uid] += contribution
				winnerTotal += contribution
			} else {
				payout += b.amount
			}
		}
	}
	refundAll := false
	if winnerTotal == 0.0 {
		// Nobody wins!  So everyone gets refunded.
		slog.Info("Nobody won the shiny bet")
		message += "\nNo winning bets!  No changes to user balances."
		refundAll = true
	}
	// Step 2: Resolve all bets, so users can use that balance for other bets.
	for _, b := range bs {
		user, err := e.core.GetUser(b.uid)
		if err != nil {
			continue
		}
		loss := true
		if b.bet.Direction == LESS && e.current < b.bet.Phase {
			loss = false
		} else if b.bet.Direction == GREATER && e.current > b.bet.Phase {
			loss = false
		} else if b.bet.Direction == EQUAL && e.current == b.bet.Phase {
			loss = false
		}
		if refundAll {
			loss = false
		}
		if loss {
			userDelta[b.uid] = -b.amount
		}
		if err := user.Resolve(transaction, b.amount, loss); err != nil {
			slog.Warn(fmt.Sprintf("Could not resolve a users bet in Close(): %v", err))
			continue
		}
	}
	// Step 3: Distribute earnings to the winners.
	if winnerTotal != 0.0 {
		fPayout := float64(payout)
		for uid, contribution := range userContributions {
			user, err := e.core.GetUser(uid)
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
			user.Earn(transaction, amount)
		}
	}
	if err := transaction.Commit(); err != nil {
		return err
	}

	// In a separate transaction, refresh balances of people who got too low, so
	// they can continue to play.
	if err := e.core.RefreshBalance(); err != nil {
		return err
	}

	// TODO: This should probably be an embed for better UI.  I am not a UI guy.
	// Sends a message to notify balance changes due to event close
	if e.channel != "" {
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
			user, _ := e.core.GetUser(d.uid)
			balance, _, _ := user.Balance()
			message += fmt.Sprintf("\n * <@%s>: %+d (new balance %d cakes)", d.uid, d.amount, balance)
		}
		if _, err := e.session.ChannelMessageSendComplex(e.channel, &discordgo.MessageSend{
			Content: message,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				// Let's the user be tagged by ID so their name appears without
				// pinging them everytime anyone uses the leaderboard command.
				Parse: []discordgo.AllowedMentionType{},
			},
		}); err != nil {
			// don't make this error block anything
			slog.Warn(fmt.Sprintf("error sending message on shiny event close: %v", err))
		}
	}
	return nil
}

func (e *ShinyEvent) bets() ([]*internalBet, error) {
	rows, err := e.core.Database.LoadBets(shinyEventName)
	if err != nil {
		return nil, fmt.Errorf("could not load shiny bets: %v", err)
	}
	bs := make([]*internalBet, 0)
	for rows.Next() {
		var uid string
		var eid string
		var placed string
		var amount int
		var risk float64
		var bet string
		if err := rows.Scan(&uid, &eid, &placed, &amount, &risk, &bet); err != nil {
			slog.Warn(fmt.Sprintf("unable to scan bet row: %s", err))
			continue
		}
		placedTs, err := time.Parse(time.DateTime, placed)
		if err != nil {
			slog.Warn(fmt.Sprintf("unable to parse bet placed time: %s", err))
			continue
		}
		bs = append(bs, &internalBet{
			amount: amount,
			bet:    betFrom(bet),
			placed: placedTs,
			risk:   risk,
			uid:    uid,
		})
	}
	return bs, nil
}

func (e *ShinyEvent) Wager(uid string, amount int, placed time.Time, inputBet any) (any, error) {
	wagerReqs.Inc()
	if !e.open {
		return nil, fmt.Errorf("betting is closed")
	}
	bet, ok := inputBet.(Bet)
	if !ok {
		return nil, fmt.Errorf("fourth argument must be of type Bet")
	}
	r, err := e.risk(bet)
	if err != nil {
		return nil, err
	}
	user, err := e.core.GetUser(uid)
	if err != nil {
		return nil, err
	}
	transaction, err := e.core.Database.OpenTransaction()
	if err != nil {
		return nil, err
	}

	if err := user.Reserve(transaction, amount); err != nil {
		return nil, err
	}
	if err := transaction.WriteBet(uid, "shiny", placed, amount, r, bet.storage()); err != nil {
		return nil, err
	}
	if err := transaction.Commit(); err != nil {
		return nil, err
	}
	wagerSuccess.Inc()
	return PlacedBet{Amount: amount, Risk: r}, nil
}

type PhaseLengthError struct {
}

func (p *PhaseLengthError) Error() string {
	return "predicted phase cannot be less than the current phase"
}

func (e *ShinyEvent) risk(bet Bet) (float64, error) {
	// Lock to ensure a consistent view of current phase during risk calc.
	e.mu.Lock()
	defer e.mu.Unlock()
	if bet.Phase <= e.current {
		return 0.0, &PhaseLengthError{}
	}
	psp := math.Pow(8191.0/8192.0, float64(bet.Phase-e.current))
	// PSP is the chance that the phase will be longer than what the user has
	// guessed.  For a < bet, that's risk.  For a > bet, that's the inverse of
	// risk.
	if bet.Direction == GREATER {
		return 1 - psp, nil
	}
	if bet.Direction == EQUAL {
		// Effectively multiply by 8192/8191 to subtract 1 from the length used
		// to calculate psp, then multiply by 1/8192 for the shiny happening
		// exactly on the predicted phase.  That is the probability that it is
		// exactly that phase, so "1 -" to turn it into risk
		return 1.0 - (psp / 8191.0), nil
	}
	return psp, nil
}

func (e *ShinyEvent) Interpret(blob string) string {
	bet := betFrom(blob)
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
