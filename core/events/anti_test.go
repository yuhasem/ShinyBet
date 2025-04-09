// Don't worry, anti_test will NOT untest your code.
package events

import (
	"bet/core"
	"bet/core/db"
	"testing"
	"time"
)

func TestSaveAndLoadAntiEvent(t *testing.T) {
	d := db.Fake()
	c := core.New(d, nil)
	e := &AntiShinyEvent{
		phaseLifecycle: &phaseLifecycle{
			eventId:     antiEventName,
			probability: 0.5,
			core:        c,
			current:     1000,
		},
		c:                  c,
		lastAntiEncounters: 200,
	}
	if err := e.writeDetails(); err != nil {
		t.Errorf("unexpected error writing details: %v", err)
	}
	tx, _ := d.OpenTransaction()
	tx.WriteOpened(antiEventName, time.Date(2025, time.March, 1, 0, 0, 1, 0, time.UTC))
	tx.WriteClosed(antiEventName, time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC))
	tx.Commit()
	// Change the event's current/encounters to know it actually loaded
	e.phaseLifecycle.current = 0
	e.lastAntiEncounters = 0
	loadAntiEvent(e)
	if e.phaseLifecycle.current != 1000 {
		t.Errorf("loaded current %d, expected 1000", e.phaseLifecycle.current)
	}
	if e.lastAntiEncounters != 200 {
		t.Errorf("loaded encounters %d, expected 200", e.lastAntiEncounters)
	}
}
