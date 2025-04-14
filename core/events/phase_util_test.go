package events

import (
	"bet/core"
	"bet/core/db"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestPhaseBet(t *testing.T) {
	for _, tc := range []struct {
		bet PhaseBet
	}{
		{
			bet: PhaseBet{Direction: LESS, Phase: 100},
		},
		{
			bet: PhaseBet{Direction: GREATER, Phase: 101},
		},
		{
			bet: PhaseBet{Direction: EQUAL, Phase: 24242424242},
		},
	} {
		got := phaseBetFrom(tc.bet.storage())
		if got.Direction != tc.bet.Direction {
			t.Errorf("store+read direction is %d, want %d", got.Direction, tc.bet.Direction)
		}
		if got.Phase != tc.bet.Phase {
			t.Errorf("store+read phase is %d, want %d", got.Phase, tc.bet.Phase)
		}
	}
}

type FakeSession struct {
	SendCount int
}

func (f *FakeSession) ChannelMessageSendComplex(string, *discordgo.MessageSend, ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.SendCount++
	return nil, nil
}

func TestPhaseOpen(t *testing.T) {
	for _, tc := range []struct {
		open        time.Time
		state       int
		current     int
		wantError   error
		wantWrite   string
		wantState   int
		wantCurrent int
	}{
		{
			open:        time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
			state:       CLOSING,
			current:     100,
			wantError:   StateMachineError{expected: CLOSED, actual: CLOSING},
			wantState:   CLOSING,
			wantCurrent: 100,
		},
		{
			open:        time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
			state:       CLOSED,
			current:     100,
			wantWrite:   "2020-01-01 00:00:00",
			wantState:   OPEN,
			wantCurrent: 0,
		},
	} {
		d := db.Fake()
		s := &FakeSession{}
		core := core.New(d, s)
		l := phaseLifecycle{
			eventId: "test",
			core:    core,
			// Normally not initializaed by callers
			state:   tc.state,
			current: tc.current,
		}
		gotErr := l.Open(tc.open)
		if gotErr != tc.wantError {
			t.Errorf("Open(...) returned %s, want %s", gotErr, tc.wantError)
		}
		if l.state != tc.wantState {
			t.Errorf("Open(...) made state %d, want %d", l.state, tc.wantState)
		}
		if l.current != tc.wantCurrent {
			t.Errorf("Open(...) made current %d, want %d", l.current, tc.wantCurrent)
		}
		row, _ := d.LoadEvent("test")
		for row.Next() {
			var id string
			var open string
			var close string
			var details string
			row.Scan(&id, &open, &close, &details)
			if open != tc.wantWrite {
				t.Errorf("Open(...) wrote open %s, want %s", open, tc.wantWrite)
			}
		}
	}
}

func TestPhaseClose(t *testing.T) {
	for _, tc := range []struct {
		close     time.Time
		state     int
		wantError error
		wantWrite string
		wantState int
	}{
		{
			close:     time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
			state:     CLOSED,
			wantError: StateMachineError{expected: OPEN, actual: CLOSED},
			wantState: CLOSED,
		},
		{
			close:     time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC),
			state:     OPEN,
			wantWrite: "2020-01-01 00:00:00",
			wantState: CLOSING,
		},
	} {
		d := db.Fake()
		s := &FakeSession{}
		core := core.New(d, s)
		l := phaseLifecycle{
			eventId: "test",
			core:    core,
			// Normally not initializaed by callers
			state: tc.state,
		}
		gotErr := l.Close(tc.close)
		if gotErr != tc.wantError {
			t.Errorf("Open(...) returned %s, want %s", gotErr, tc.wantError)
		}
		if l.state != tc.wantState {
			t.Errorf("Open(...) made state %d, want %d", l.state, tc.wantState)
		}
		row, _ := d.LoadEvent("test")
		for row.Next() {
			var id string
			var open string
			var close string
			var details string
			row.Scan(&id, &open, &close, &details)
			if close != tc.wantWrite {
				t.Errorf("Open(...) wrote open %s, want %s", close, tc.wantWrite)
			}
		}
	}
}

func TestPhaseResolve(t *testing.T) {
	d := db.Fake()
	s := &FakeSession{}
	c := core.New(d, s)
	l := phaseLifecycle{
		eventId:     "test",
		probability: 0.5,
		core:        c,
		channel:     "not empty",
		state:       OPEN,
		current:     0,
	}

	betTime := time.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC)
	l.Wager("user1", 100, betTime, PhaseBet{Direction: LESS, Phase: 2})    // loss
	l.Wager("user1", 100, betTime, PhaseBet{Direction: LESS, Phase: 5})    // risk 0.0625
	l.Wager("user2", 10, betTime, PhaseBet{Direction: EQUAL, Phase: 3})    // risk 0.875
	l.Wager("user3", 100, betTime, PhaseBet{Direction: GREATER, Phase: 4}) // loss

	l.Update(3)
	err := l.Close(time.Date(2020, time.January, 2, 0, 0, 1, 0, time.UTC))
	if err != nil {
		t.Errorf("Close() returned unexpected error: %v", err)
	}

	// payout 200, winner total = 15.0 (u1 -> 6.25, u2 -> 8.75)
	// u1 loses 100 gains 84 net -16
	// u2 gains 117
	// u3 loses 100
	err = l.Resolve()
	if err != nil {
		t.Errorf("Resolve() returned unexpected error: %v", err)
	}
	u1, _ := c.GetUser("user1")
	bal1, _, _ := u1.Balance()
	if bal1 != 984 {
		t.Errorf("user1 has %d balance, expected 984", bal1)
	}
	u2, _ := c.GetUser("user2")
	bal2, _, _ := u2.Balance()
	if bal2 != 1117 {
		t.Errorf("user2 has %d balance, expected 1117", bal2)
	}
	u3, _ := c.GetUser("user3")
	bal3, _, _ := u3.Balance()
	if bal3 != 900 {
		t.Errorf("user3 has %d balance, expected 900", bal3)
	}
	if s.SendCount != 1 {
		t.Errorf("Expected 1 message to be sent, instead got %d", s.SendCount)
	}
}

func TestPhaseResolveNoWiners(t *testing.T) {
	d := db.Fake()
	s := &FakeSession{}
	c := core.New(d, s)
	l := phaseLifecycle{
		eventId:     "test",
		probability: 0.5,
		core:        c,
		channel:     "not empty",
		state:       OPEN,
		current:     0,
	}

	// Setup the condition by creating a bet which will lose,
	b := PhaseBet{Direction: LESS, Phase: 5}
	l.Wager("user", 100, time.Date(2020, time.January, 2, 0, 0, 0, 0, time.UTC), b)
	// First, assert that Core has registered the user as having cakes in bets.
	u, _ := c.GetUser("user")
	_, inBets, _ := u.Balance()
	if inBets == 0 {
		t.Error("User 'user' should have cakes in bets but has 0")
	}

	// Setup the condition for the loss
	l.Update(10)
	err := l.Close(time.Date(2020, time.January, 2, 0, 0, 1, 0, time.UTC))
	if err != nil {
		t.Errorf("Close() returned unexpected error: %v", err)
	}

	// Since there were no winning bets, we expect this to refund the bet.
	err = l.Resolve()
	if err != nil {
		t.Errorf("Resolve() returned unexpected error: %v", err)
	}
	_, inBets, _ = u.Balance()
	if inBets != 0 {
		t.Errorf("User 'user' should have no cakes in bets but has %d", inBets)
	}
	if s.SendCount != 1 {
		t.Errorf("1 message should have been sent over the session, instead %d were", s.SendCount)
	}
}

func TestPhaseResolveNoChannel(t *testing.T) {
	d := db.Fake()
	s := &FakeSession{}
	c := core.New(d, s)
	l := phaseLifecycle{
		eventId:     "test",
		probability: 0.5,
		core:        c,
		channel:     "",
		state:       OPEN,
		current:     0,
	}

	l.Wager("user", 1000, time.Now(), PhaseBet{Direction: LESS, Phase: 10})
	l.Update(1)
	l.Close(time.Now())
	l.Resolve()
	if s.SendCount != 0 {
		t.Errorf("Expected 0 messages to be sent, instead %d were", s.SendCount)
	}
}
