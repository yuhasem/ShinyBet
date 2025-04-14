package events

// import (
// 	"bet/core"
// 	"bet/core/db"
// 	"fmt"
// 	"strings"
// 	"testing"
// 	"time"
// )

// func TestWager(t *testing.T) {
// 	for _, tc := range []struct {
// 		userId    string
// 		amount    int
// 		direction int
// 		value     int
// 		wantErr   error
// 	}{
// 		{
// 			// A non-existent gets created and the bet is placed.
// 			userId: "not-user",
// 			amount: 30,
// 			value:  150,
// 		},
// 		{
// 			// Cannot place a non-positive bet.
// 			userId:  "user",
// 			amount:  0,
// 			value:   150,
// 			wantErr: fmt.Errorf("non-positive err"),
// 		},
// 		{
// 			// Cannot place a bet that would resolve immediately.
// 			userId:    "user",
// 			amount:    30,
// 			direction: LESS,
// 			value:     99,
// 			wantErr:   fmt.Errorf("0%% risk bet err"),
// 		},
// 		{
// 			// Cannot place a bet that would resolve immediately.
// 			userId:    "user",
// 			amount:    30,
// 			direction: GREATER,
// 			value:     99,
// 			wantErr:   fmt.Errorf("100%% risk bet err"),
// 		},
// 		{
// 			// Cannot place a bet for more money than you have.
// 			userId:  "user",
// 			amount:  1001,
// 			value:   150,
// 			wantErr: fmt.Errorf("over money bet err"),
// 		},
// 	} {
// 		c := core.New(db.Fake())
// 		c.GetUser("user")
// 		e := NewShinyEvent(c, nil, "")
// 		e.Open(time.Now())
// 		e.Update(100)
// 		_, err := e.Wager(tc.userId, tc.amount, time.Time{}, Bet{Direction: tc.direction, Phase: tc.value})
// 		if (err == nil) != (tc.wantErr == nil) {
// 			t.Errorf("Unexpected error, got %v want %v", err, tc.wantErr)
// 		}
// 	}
// }

// func TestClose(t *testing.T) {
// 	c := core.New(db.Fake())
// 	e := NewShinyEvent(c, nil, "")
// 	open, _ := time.Parse(time.DateTime, "2025-01-01 00:00:00")
// 	e.Open(open)
// 	e.Wager("user1", 50, open.Add(time.Second), Bet{Direction: GREATER, Phase: 5678}) // odds: ~0.5
// 	e.Wager("user1", 50, open.Add(time.Second), Bet{Direction: LESS, Phase: 11356})   // odds: ~0.25
// 	u1, _ := c.GetUser("user1")
// 	if _, inBets, _ := u1.Balance(); inBets != 100 {
// 		t.Errorf("user 1 should have 100 in bets but has %d", inBets)
// 	}
// 	e.Wager("user2", 50, open.Add(time.Second), Bet{Direction: GREATER, Phase: 11356}) // odds: ~0.75
// 	e.Update(4000)
// 	nextBetTime, _ := time.Parse(time.DateTime, "2025-01-01 09:36:00")
// 	e.Wager("user2", 20, nextBetTime, Bet{Direction: GREATER, Phase: 9678}) // odds: ~0.5
// 	u2, _ := c.GetUser("user2")
// 	if _, inBets, _ := u2.Balance(); inBets != 70 {
// 		t.Errorf("user 2 should have 70 in bets but has %d", inBets)
// 	}
// 	e.Update(10000)
// 	close, _ := time.Parse(time.DateTime, "2025-01-02 00:00:00")
// 	e.Close(close)
// 	bal, inBets, _ := u1.Balance()
// 	if bal != 1040 {
// 		t.Errorf("user1 should have 1040 balance after payout, has %d", bal)
// 	}
// 	if inBets != 0 {
// 		t.Errorf("user 1 should have 0 in bets but has %d", inBets)
// 	}
// 	bal, inBets, _ = u2.Balance()
// 	if bal != 961 {
// 		t.Errorf("user2 should have 961 balance after payout, has %d", bal)
// 	}
// 	if inBets != 0 {
// 		t.Errorf("user 1 should have 0 in bets but has %d", inBets)
// 	}
// }

// func TestSummary(t *testing.T) {
// 	c := core.New(db.Fake())
// 	e := NewShinyEvent(c, nil, "")
// 	open, _ := time.Parse(time.DateTime, "2025-01-01 00:00:00")
// 	e.Open(open)
// 	// Resolved wagers
// 	e.Wager("user1", 100, open.Add(time.Second), Bet{Direction: GREATER, Phase: 420}) // risk ~0.05
// 	e.Wager("user2", 100, open.Add(time.Second), Bet{Direction: GREATER, Phase: 507}) // risk ~0.06
// 	e.Wager("user3", 100, open.Add(time.Second), Bet{Direction: EQUAL, Phase: 500})
// 	e.Wager("user4", 100, open.Add(time.Second), Bet{Direction: LESS, Phase: 500})
// 	// Unresolved wagers
// 	e.Wager("user1", 100, open.Add(time.Second), Bet{Direction: LESS, Phase: 11356})   // risk ~0.25
// 	e.Wager("user5", 100, open.Add(time.Second), Bet{Direction: GREATER, Phase: 5678}) // risk ~0.5
// 	e.Update(1000)

// 	summary, err := e.BetsSummary("risk")
// 	if err != nil {
// 		t.Errorf("unexpected error in BetsSummary(): %v", err)
// 	}
// 	want := "There are 600 cakes in bets on the shiny event."
// 	if !strings.Contains(summary, want) {
// 		t.Errorf("BetsSummary() = %s, wanted total cakes like %s", summary, want)
// 	}
// 	want = "200 cakes are guaranteed"
// 	if !strings.Contains(summary, want) {
// 		t.Errorf("BetsSummary() = %s, wanted guaranteed payout like %s", summary, want)
// 	}
// 	want = "200 cakes are in unresolved bets"
// 	if !strings.Contains(summary, want) {
// 		t.Errorf("BetsSummary() = %s, wanted guaranteed payout like %s", summary, want)
// 	}
// 	want = "11.00 is the risk adjusted pool"
// 	if !strings.Contains(summary, want) {
// 		t.Errorf("BetsSummary() = %s, wanted guarantted winners like %s", summary, want)
// 	}
// 	want = " * <@user5> placed 100 cakes on phase > 5678 (50.00% risk)"
// 	if !strings.Contains(summary, want) {
// 		t.Errorf("BetsSummary() = %s, wanted bet line like %s", summary, want)
// 	}
// 	want = " * <@user1> placed 100 cakes on phase < 11356 (25.00% risk)"
// 	if !strings.Contains(summary, want) {
// 		t.Errorf("BetsSummary() = %s, wanted bet line like %s", summary, want)
// 	}
// }
