package events

import (
	"bet/core"
	"bet/core/db"
	"testing"
	"time"
)

func TestSaveAndLoadShinyEvent(t *testing.T) {
	d := db.Fake()
	c := core.New(d, nil)
	e := &ShinyEvent{
		phaseLifecycle: &phaseLifecycle{
			eventId:     shinyEventName,
			probability: 0.5,
			core:        c,
			current:     1000,
		},
		core: c,
	}
	if err := e.writeDetails(); err != nil {
		t.Errorf("unexpected error writing details: %v", err)
	}
	// Also need open/close written to read back.  Phase lifecycle already tests
	// those writes.
	tx, _ := d.OpenTransaction()
	tx.WriteOpened(shinyEventName, time.Date(2025, time.March, 1, 1, 0, 0, 0, time.UTC))
	tx.WriteClosed(shinyEventName, time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC))
	tx.Commit()
	// Change event's current to ensure it really is loading.
	e.phaseLifecycle.current = 0
	loadEvent(e)
	if e.phaseLifecycle.current != 1000 {
		t.Errorf("after load event had current %d, want 1000", e.phaseLifecycle.current)
	}
	if e.phaseLifecycle.state != OPEN {
		t.Errorf("after load event state was %d, want %d", e.phaseLifecycle.state, OPEN)
	}
}
