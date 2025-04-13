package events

import (
	"bet/core"
	"bet/state"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const shinyEventName = "shiny"

type ShinyEvent struct {
	*phaseLifecycle
	core *core.Core
	// Not persisted, but kept in memory to check an error condition.
	lastEncounterWasShiny bool
}

func NewShinyEvent(c *core.Core, s *discordgo.Session, channel string) *ShinyEvent {
	e := &ShinyEvent{
		phaseLifecycle: &phaseLifecycle{
			eventId:     shinyEventName,
			probability: 8191.0 / 8192.0,
			core:        c,
			channel:     channel,
		},
		core: c,
	}
	// Attempt to re-construct state by loading it from storage. Regardless of
	// the outcome, the event is the canonical one we want to register with
	// core.
	loadEvent(e)
	return e
}

func loadEvent(event *ShinyEvent) {
	rows, err := event.core.Database.LoadEvent(shinyEventName)
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
		if !closeTs.After(openTs) {
			event.phaseLifecycle.state = OPEN
		}
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
		if err := e.Resolve(); err != nil {
			slog.Error(fmt.Sprintf("error resolving shiny event: %v", err))
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

func (e *ShinyEvent) Interpret(blob string) string {
	bet := betFrom(blob)
	return interpretBet(bet)
}

func interpretBet(bet Bet) string {
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

func (e *ShinyEvent) BetsSummary(style string) (string, error) {
	bets, err := e.bets()
	if err != nil {
		return "", nil
	}
	// First collect the bets into 3 groups.  Unresolved, guaranteed win, and
	// guaranteed loss.
	unresolvedBets := make([]*internalBet, 0, len(bets))
	var inLosers int
	var inWinners float64
	// Also collect total amount of currency on the bet.
	var total int
	var inUnresolved int
	for _, b := range bets {
		total += b.amount
		if b.bet.Phase >= e.current {
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
	message := fmt.Sprintf("There are %d cakes in bets on the %s event.\n", total, shinyEventName)
	message += fmt.Sprintf(" * %d cakes are guaranteed to be in the payout\n", inLosers)
	message += fmt.Sprintf(" * %d cakes are in unresolved bets\n", inUnresolved)
	message += fmt.Sprintf(" * %.2f is the risk adjusted pool of guaranteed winners\n", inWinners)
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
		message += fmt.Sprintf("\n * <@%s> placed %d cakes on %s (%.2f%% risk)", b.uid, b.amount, interpretBet(b.bet), b.risk*100)
	}
	return message, nil
}

func sortByAdjustedRisk(a, b *internalBet) int {
	diff := float64(b.amount)*b.risk - float64(a.amount)*a.risk
	if diff > 0 {
		return 1
	} else if diff < 0 {
		return -1
	}
	return 0
}

func sortByUpcoming(a, b *internalBet) int {
	return a.bet.Phase - b.bet.Phase
}
