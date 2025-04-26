package events

import (
	"bet/core/db"
	"fmt"
	"slices"
	"time"
)

type EventState int

// EventState enum
const (
	CLOSED = iota
	OPEN
	CLOSING
)

type StateMachineError struct {
	expected EventState
	actual   EventState
}

func (err StateMachineError) Error() string {
	return fmt.Sprintf("wrong state for transition, expected %d, was %d", err.expected, err.actual)
}

// Opens an event by writing an open time to the database, and returns the state
// the event should be in after this action, plus an error if anything
// unrecoverable happened.
func commonOpen(d db.Database, eid string, open time.Time, state EventState) (EventState, error) {
	if state != CLOSED {
		return state, StateMachineError{expected: CLOSED, actual: state}
	}
	tx, err := d.OpenTransaction()
	if err != nil {
		return state, err
	}
	if err := tx.WriteOpened(eid, open); err != nil {
		return state, err
	}
	if err := tx.Commit(); err != nil {
		return state, err
	}
	return OPEN, nil
}

// Closes an event by writing a close time to the database, and returns the
// state the event should be in after this action, plus and error if anything
// unrecoverable happened.
func commonClose(d db.Database, eid string, close time.Time, state EventState) (EventState, error) {
	if state != OPEN {
		return state, StateMachineError{expected: OPEN, actual: state}
	}
	tx, err := d.OpenTransaction()
	if err != nil {
		return state, err
	}
	if err := tx.WriteClosed(eid, close); err != nil {
		return state, err
	}
	if err := tx.Commit(); err != nil {
		return state, err
	}
	return CLOSING, nil
}

type delta struct {
	uid    string
	amount int
}

func (d delta) String() string {
	return fmt.Sprintf("<@%s> %+d", d.uid, d.amount)
}

func deltaMapToList(userDelta map[string]int) []delta {
	d := make([]delta, 0, len(userDelta))
	for uid, diff := range userDelta {
		d = append(d, delta{uid: uid, amount: diff})
	}
	return d
}

func sortedDeltas(userDelta map[string]int) []delta {
	deltas := deltaMapToList(userDelta)
	slices.SortFunc(deltas, func(a, b delta) int {
		return b.amount - a.amount
	})
	return deltas
}
