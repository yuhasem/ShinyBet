// phase_util contains common definitions for events which rely on a stream of
// events until one happens with a specific probability.  This is primarily
// focused on serving the "shiny" and "antishiny" events.
package events

import (
	"bet/core"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

type PhaseBet struct {
	Direction int
	Phase     int
}

// Direction enum
const (
	LESS = iota
	EQUAL
	GREATER
)

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

// PhaseBet implements lifecycle management methods (Open, Update, and Close)
// and the command method Wager, to be used in composing phase events.
type phaseBet struct {
	// The eventId used to read from and write to the database.
	eventId string
	// The probability of the betting event occurring at each encounter.
	probability float64
	// A reference to the Core to use for databse connections.
	core *core.Core

	// The following don't need to be initialized.
	mu      sync.Mutex
	open    bool
	current int
	opened  time.Time
}

// Open updates the database for the open time and resets state for tracking the
// phase.  It is safe to call Open while the event is already open.
func (p *phaseBet) Open(open time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.open {
		return nil
	}
	return nil
}

func (p *phaseBet) Update(phase int) {

}

func (p *phaseBet) Close(close time.Time) error {
	return nil
}

func (p *phaseBet) Wager(uid string, amount int, placed time.Time, bet any) (any, error) {
	return nil, nil
}
