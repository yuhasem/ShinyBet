package core

import (
	"bet/core/db"
	"testing"
	"time"
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

// TODO: actually mock these.
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

type FakeCron struct {
	notify chan bool
}

func (f *FakeCron) ID() string { return "fake-cron" }

func (f *FakeCron) After() time.Duration {
	return time.Second
}

func (f *FakeCron) Run() error {
	f.notify <- true
	return nil
}

func TestCron(t *testing.T) {
	d := db.Fake()
	clock := NewFakeClock(time.Time{})
	c := New(d, nil, clock)
	notify := make(chan bool)
	c.AddCron(&FakeCron{
		notify: notify,
	})
	// Test the initial wait in `schedule`
	clock.Set(time.Time{}.Add(time.Second))
	<-notify
	// Test the wait inside the loop in `schedule`
	clock.Set(time.Time{}.Add(2 * time.Second))
	<-notify
	c.Close()
}
