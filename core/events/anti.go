package events

import (
	"bet/core"
	"bet/state"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const antiEventName = "anti"

type AntiShinyEvent struct {
	*phaseLifecycle
	c *core.Core
	// Phase isn't tracked or reported by Pokebot, so we keep track of the total
	// encounters during the last anti-shiny and compute the phase ourselves.
	lastAntiEncounters int
}

func NewAntiShinyEvent(c *core.Core, channel string) *AntiShinyEvent {
	e := &AntiShinyEvent{
		phaseLifecycle: &phaseLifecycle{
			eventId:     antiEventName,
			displayName: "Anti Shiny",
			probability: 1.0 / 8192.0,
			core:        c,
			channel:     channel,
		},
		c: c,
	}
	loadAntiEvent(e)
	return e
}

func loadAntiEvent(event *AntiShinyEvent) {
	row, err := event.core.Database.LoadEvent(antiEventName)
	if err != nil {
		slog.Error(fmt.Sprintf("could not load anti event from db: %v", err))
	}
	gotRow := false
	for row.Next() {
		gotRow = true
		var eid string
		var lastOpen string
		var lastClose string
		var details string
		if err := row.Scan(&eid, &lastOpen, &lastClose, &details); err != nil {
			slog.Error(fmt.Sprintf("could not scan anti event row: %v", err))
			continue
		}
		openTs, err := time.Parse(time.DateTime, lastOpen)
		if err != nil {
			slog.Error(fmt.Sprintf("could not parse open time: %v", err))
			continue
		}
		closeTs, err := time.Parse(time.DateTime, lastClose)
		if err != nil {
			slog.Error(fmt.Sprintf("could not parse close time: %v", err))
			continue
		}
		phase, encounters, err := parseDetails(details)
		if err != nil {
			slog.Error(fmt.Sprintf("could not parse details: %v", err))
			continue
		}
		if !closeTs.After(openTs) {
			event.phaseLifecycle.state = OPEN
			event.phaseLifecycle.current = phase
			event.lastAntiEncounters = encounters
		}
	}
	if gotRow {
		return
	}
	// Write a new base row.
	slog.Debug("no existing anti row")
	tx, err := event.core.Database.OpenTransaction()
	if err != nil {
		slog.Error(fmt.Sprintf("could not open transaction to write new anti row: %v", err))
		return
	}
	if err := tx.WriteNewEvent(antiEventName, time.Now(), "0,0"); err != nil {
		slog.Error(fmt.Sprintf("could not write new anti row: %v", err))
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Error(fmt.Sprintf("could not commit anti row: %v", err))
		return
	}
	// Note: We haven't seen an anti to know the total encounters which makes
	// first phase INCREDIBLY jank.  It's still technically functional, because
	// the phase lifecycle is forward looking for bets, but users may have to
	// bet very large numbers if the anti phase is first enabled after the bot
	// has been running for a while.
	if err := event.Open(time.Now()); err != nil {
		slog.Error(fmt.Sprintf("error opening anti event: %v", err))
		return
	}
}

func parseDetails(details string) (phase, encounters int, err error) {
	s := strings.Split(details, ",")
	if len(s) != 2 {
		return 0, 0, fmt.Errorf("not enough parts in anti details")
	}
	phase, err = strconv.Atoi(s[0])
	if err != nil {
		return
	}
	encounters, err = strconv.Atoi(s[1])
	if err != nil {
		return
	}
	return
}

func (e *AntiShinyEvent) Notify(s *state.State) {
	slog.Debug("start anti notify")
	e.Update(s.Stats.Totals.TotalEncounters - e.lastAntiEncounters)
	if s.Encounter.IsAntiShiny {
		slog.Debug(fmt.Sprintf("received state: %+v", s))
		e.lastAntiEncounters = s.Stats.Totals.TotalEncounters
		if err := e.Close(time.Now()); err != nil {
			slog.Error(fmt.Sprintf("error closing anti event: %v", err))
		}
		if err := e.Resolve(); err != nil {
			slog.Error(fmt.Sprintf("error resolving anti event: %v", err))
		}
		if err := e.Open(time.Now()); err != nil {
			slog.Error(fmt.Sprintf("error opening anti event: %v", err))
		}
	}
	if err := e.writeDetails(); err != nil {
		slog.Error(fmt.Sprintf("error writing details to anti event: %v", err))
	}
	slog.Debug("end anti notify")
}

func (e *AntiShinyEvent) writeDetails() error {
	tx, err := e.core.Database.OpenTransaction()
	if err != nil {
		return fmt.Errorf("in open tx: %v", err)
	}
	if err := tx.WriteEventDetails(antiEventName, fmt.Sprintf("%d,%d", e.phaseLifecycle.current, e.lastAntiEncounters)); err != nil {
		return fmt.Errorf("in write: %v", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("in commit: %v", err)
	}
	return nil
}
