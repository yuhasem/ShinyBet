package events

import (
	"bet/core"
	"bet/core/db"
	"fmt"
	"testing"
	"time"
)

func TestWager(t *testing.T) {
	for _, tc := range []struct {
		userId    string
		amount    int
		direction int
		value     int
		wantErr   error
	}{
		{
			// A non-existent gets created and the bet is placed.
			userId: "not-user",
			amount: 30,
			value:  150,
		},
		{
			// Cannot place a non-positive bet.
			userId:  "user",
			amount:  0,
			value:   150,
			wantErr: fmt.Errorf("non-positive err"),
		},
		{
			// Cannot place a bet that would resolve immediately.
			userId:    "user",
			amount:    30,
			direction: LESS,
			value:     99,
			wantErr:   fmt.Errorf("0%% risk bet err"),
		},
		{
			// Cannot place a bet that would resolve immediately.
			userId:    "user",
			amount:    30,
			direction: GREATER,
			value:     99,
			wantErr:   fmt.Errorf("100%% risk bet err"),
		},
		{
			// Cannot place a bet for more money than you have.
			userId:  "user",
			amount:  1001,
			value:   150,
			wantErr: fmt.Errorf("over money bet err"),
		},
	} {
		c := core.New(db.Fake())
		c.GetUser("user")
		e := NewShinyEvent(c, nil, "")
		e.Open(time.Now())
		e.Update(100)
		_, err := e.Wager(tc.userId, tc.amount, time.Time{}, Bet{Direction: tc.direction, Phase: tc.value})
		if (err == nil) != (tc.wantErr == nil) {
			t.Errorf("Unexpected error, got %v want %v", err, tc.wantErr)
		}
	}
}

func TestClose(t *testing.T) {
	c := core.New(db.Fake())
	e := NewShinyEvent(c, nil, "")
	open, _ := time.Parse(time.DateTime, "2025-01-01 00:00:00")
	e.Open(open)
	e.Wager("user1", 50, open.Add(time.Second), Bet{Direction: GREATER, Phase: 5678}) // odds: ~0.5
	e.Wager("user1", 50, open.Add(time.Second), Bet{Direction: LESS, Phase: 11356})   // odds: ~0.25
	u1, _ := c.GetUser("user1")
	if _, inBets, _ := u1.Balance(); inBets != 100 {
		t.Errorf("user 1 should have 100 in bets but has %d", inBets)
	}
	e.Wager("user2", 50, open.Add(time.Second), Bet{Direction: GREATER, Phase: 11356}) // odds: ~0.75
	e.Update(4000)
	nextBetTime, _ := time.Parse(time.DateTime, "2025-01-01 09:36:00")
	e.Wager("user2", 20, nextBetTime, Bet{Direction: GREATER, Phase: 9678}) // odds: ~0.5
	u2, _ := c.GetUser("user2")
	if _, inBets, _ := u2.Balance(); inBets != 70 {
		t.Errorf("user 2 should have 70 in bets but has %d", inBets)
	}
	e.Update(10000)
	close, _ := time.Parse(time.DateTime, "2025-01-02 00:00:00")
	e.Close(close)
	bal, inBets, _ := u1.Balance()
	if bal != 1040 {
		t.Errorf("user1 should have 1040 balance after payout, has %d", bal)
	}
	if inBets != 0 {
		t.Errorf("user 1 should have 0 in bets but has %d", inBets)
	}
	bal, inBets, _ = u2.Balance()
	if bal != 961 {
		t.Errorf("user2 should have 961 balance after payout, has %d", bal)
	}
	if inBets != 0 {
		t.Errorf("user 1 should have 0 in bets but has %d", inBets)
	}
}
