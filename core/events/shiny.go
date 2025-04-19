package events

import (
	"bet/core"
	"bet/state"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

const shinyEventName = "shiny"

type ShinyEvent struct {
	*phaseLifecycle
	core *core.Core
	// Not persisted, but kept in memory to check an error condition.
	lastEncounterWasShiny bool
}

func NewShinyEvent(c *core.Core, channel string) *ShinyEvent {
	e := &ShinyEvent{
		phaseLifecycle: &phaseLifecycle{
			eventId:     shinyEventName,
			displayName: "Shiny",
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
		var details string
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
		phase := 0
		if details != "" {
			phase, err = strconv.Atoi(details)
			if err != nil {
				slog.Warn("could not get current phase back from details %s: %v", details, err)
			}
		}
		if !closeTs.After(openTs) {
			event.phaseLifecycle.state = OPEN
			event.phaseLifecycle.current = phase
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
	e.writeDetails()
}

func (e *ShinyEvent) writeDetails() error {
	t, err := e.core.Database.OpenTransaction()
	if err != nil {
		return err
	}
	if err := t.WriteEventDetails(shinyEventName, strconv.Itoa(e.phaseLifecycle.current)); err != nil {
		return err
	}
	if err := t.Commit(); err != nil {
		return err
	}
	return nil
}
