package crons

import (
	"bet/core"
	"bet/core/db"
	"bet/core/events"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

type fakeClock struct {
	t      time.Time
	change chan time.Time
}

func NewFakeClock(start time.Time) *fakeClock {
	return &fakeClock{
		t:      start,
		change: make(chan time.Time),
	}
}

func (f *fakeClock) Now() time.Time {
	return f.t
}
func (f *fakeClock) After(d time.Duration) <-chan time.Time {
	until := f.t.Add(d)
	ret := make(chan time.Time, 1)
	for {
		select {
		case now := <-f.change:
			if !now.Before(until) {
				ret <- now
				return ret
			}
		}
	}
	ret <- time.Time{}
	return ret
}

func (f *fakeClock) Set(t time.Time) {
	f.t = t
	f.change <- t
}

type FakeSession struct {
	SendCount int
}

func (f *FakeSession) ChannelMessageSendComplex(string, *discordgo.MessageSend, ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.SendCount++
	return nil, nil
}

func TestSelfBetCron(t *testing.T) {
	d := db.Fake()
	s := &FakeSession{}
	clock := NewFakeClock(time.Time{})
	c := core.New(d, s, clock)
	user, _ := c.GetUser("test")
	shiny := events.NewShinyEvent(c, "")
	c.RegisterEvent("shiny", shiny)
	cron := NewSelfBetCron(c, "test", time.Second, "test-channel")
	c.AddCron(cron)
	clock.Set(time.Time{}.Add(time.Second))
	<-cron.done
	_, bets, _ := user.Balance()
	if bets != 1000 {
		t.Errorf("user has %d in bets, wanted 1000 (all placed on a bet)", bets)
	}
	tx, _ := d.OpenTransaction()
	user.Earn(tx, 1)
	tx.Commit()
	// Run again, but this time the bot has 1001 balance 1000 in bets.
	clock.Set(time.Time{}.Add(2 * time.Second))
	<-cron.done
	_, bets, _ = user.Balance()
	if bets != 1000 {
		t.Errorf("user has %d in bets, wanted 1000 (no new bets placed)", bets)
	}
	tx, _ = d.OpenTransaction()
	user.Earn(tx, 100)
	tx.Commit()
	// Run again, but this time the bot has 1101 balance 1000 in bets, so it
	// should bet again
	clock.Set(time.Time{}.Add(3 * time.Second))
	<-cron.done
	_, bets, _ = user.Balance()
	if bets != 1101 {
		t.Errorf("user has %d in bets, wanted 1101 (all placed again)", bets)
	}
	if s.SendCount != 2 {
		t.Errorf("got %d sent messages, want 2", s.SendCount)
	}
}
