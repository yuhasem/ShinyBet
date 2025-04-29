// self_bet implements a cron which makes the Discord user for this bot place
// random bets on the current shiny phase length.  This is because users can
// donate to the bot user, therefore they will and these cakes are lost from the
// economy.
package crons

import (
	"bet/core"
	"bet/core/events"
	"fmt"
	"math"
	"math/rand"
	"time"
)

type PhaseEvent interface {
	core.Event
	Current() int
}

type SelfBetCron struct {
	user    string
	after   time.Duration
	channel string
	core    *core.Core
	// testing only for now, but maybe Cron interface should evolve to accept
	// this kind of thing
	done chan bool
}

func NewSelfBetCron(c *core.Core, user string, after time.Duration, channel string) *SelfBetCron {
	return &SelfBetCron{
		user:    user,
		after:   after,
		channel: channel,
		core:    c,
		done:    make(chan bool, 1),
	}
}

func (c *SelfBetCron) ID() string {
	return "self-bet"
}

func (c *SelfBetCron) After() time.Duration {
	return c.after
}

func (c *SelfBetCron) Run() error {
	defer func() { c.done <- true }()
	me, err := c.core.GetUser(c.user)
	if err != nil {
		return err
	}
	balance, inBets, err := me.Balance()
	if err != nil {
		return err
	}
	// Only participate if we have more than the default balance left in unspent
	// cakes.
	if balance-inBets <= 100 {
		return nil
	}
	event, err := c.core.GetEvent("shiny")
	if err != nil {
		return err
	}
	pe, ok := event.(PhaseEvent)
	if !ok {
		return fmt.Errorf("could not cast the shiny event to a PhaseEvent")
	}
	// Pick the phase length to bet on
	psp := rand.Float64()
	length := math.Log(psp) / math.Log(8191.0/8192.0)
	// Always pick the direction with higher risk
	dir := events.LESS
	dirStr := "less"
	if length < 5678 {
		dir = events.GREATER
		dirStr = "greater"
	}
	// And finally add the length to the current phase.
	betLength := pe.Current() + int(length)
	p, err := pe.Wager(c.user, balance-inBets, time.Now(), events.PhaseBet{Direction: dir, Phase: betLength})
	if err != nil {
		return nil
	}
	placed, ok := p.(events.PlacedPhaseBet)
	if !ok {
		placed = events.PlacedPhaseBet{}
	}
	c.core.SendMessage(c.channel, fmt.Sprintf("<@%s> placed %d cakes on shiny phase being %s than %d encounters! (%.2f%% risk)", c.user, placed.Amount, dirStr, betLength, 100*placed.Risk))
	return nil
}
