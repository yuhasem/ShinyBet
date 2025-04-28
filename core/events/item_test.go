package events

import (
	"bet/core"
	"bet/core/db"
	"bet/state"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestItemNotify(t *testing.T) {
	d := db.Fake()
	s := &FakeSession{}
	c := core.New(d, s, nil)
	u, _ := c.GetUser("user")
	e := ItemEvent{
		c:       c,
		species: "do",
		item:    "what",
		state:   OPEN,
	}
	tx, _ := d.OpenTransaction()
	tx.WriteOpened(itemEventName, time.Now().Add(-time.Second))
	tx.WriteBet("user", itemEventName, time.Now(), 100, 0.0, "true")
	u.Reserve(tx, 100)
	tx.Commit()
	state := &state.State{}

	// Notify event of a state without a shiny
	r := strings.NewReader(`{
	  "encounter":{
	    "is_shiny":false,
		"species":{
		  "name":"do"
		},
		"held_item":{
		  "name":"what"
		}
	  }}`)
	if err := json.NewDecoder(r).Decode(state); err != nil {
		t.Fatalf("could not parse state json: %v", err)
	}
	e.Notify(state)
	// Expect that the bet is not resolved
	_, bets, _ := u.Balance()
	if bets != 100 {
		t.Errorf("user had %d in bets, expected 100 (no event resolution on no shiny)", bets)
	}

	// Notify event of a state with a shiny, but of the wrong species
	r = strings.NewReader(`{
	  "encounter":{
	    "is_shiny":true,
		"species":{
		  "name":"psych"
		},
		"held_item":{
		  "name":"what"
		}
	  }}`)
	if err := json.NewDecoder(r).Decode(state); err != nil {
		t.Fatalf("could not parse state json: %v", err)
	}
	e.Notify(state)
	// Expect that bet is not resolved
	_, bets, _ = u.Balance()
	if bets != 100 {
		t.Errorf("user had %d in bets, expected 100 (no event resolution on wrong species)", bets)
	}

	// Notify event of a state with the shiny and item (with different case).
	r = strings.NewReader(`{
	  "encounter":{
	    "is_shiny":true,
		"species":{
		  "name":"Do"
		},
		"held_item":{
		  "name":"whaT"
		}
	  }}`)
	if err := json.NewDecoder(r).Decode(state); err != nil {
		t.Fatalf("could not parse state json: %v", err)
	}
	e.Notify(state)
	// Expect that the bet IS resolved
	_, bets, _ = u.Balance()
	if bets != 0 {
		t.Errorf("user had %d in bets, expected 0 (event resolved)", bets)
	}
	if e.state != CLOSED {
		t.Errorf("item event in state %d, expected %d", e.state, CLOSED)
	}
}

func TestItemWager(t *testing.T) {
	d := db.Fake()
	s := &FakeSession{}
	c := core.New(d, s, nil)
	e := ItemEvent{
		c:     c,
		prob:  0.05,
		state: OPEN,
	}
	risk, err := e.Wager("user1", 100, time.Now(), true)
	if err != nil {
		t.Errorf("Unexpected error in wager: %v", err)
	}
	r, ok := risk.(float64)
	if !ok {
		t.Errorf("risk returned from wager is not a float64")
	}
	if r != 0.95 {
		t.Errorf("wager returned display risk %f, wanted 0.05", r)
	}
	risk, _ = e.Wager("user2", 800, time.Now(), false)
	r = risk.(float64)
	if r != 0.05 {
		t.Errorf("wager returned display risk %f, wanted 0.95", r)
	}
	e.Wager("user3", 200, time.Now(), false)
	e.Update(false)
	e.Close(time.Now())
	e.Resolve()
	u1, _ := c.GetUser("user1")
	bal, inBets, _ := u1.Balance()
	if bal != 900 {
		t.Errorf("user1 has %d in balance, expected 900", bal)
	}
	if inBets != 0 {
		t.Errorf("user1 has %d in bets, expected 0", inBets)
	}
	u2, _ := c.GetUser("user2")
	bal, inBets, _ = u2.Balance()
	if bal != 1080 {
		t.Errorf("user2 has %d in balance, expected 1080", bal)
	}
	if inBets != 0 {
		t.Errorf("user2 has %d in bets, expected 0", inBets)
	}
	u3, _ := c.GetUser("user3")
	bal, inBets, _ = u3.Balance()
	if bal != 1020 {
		t.Errorf("user3 has %d in balance, expected 1020", bal)
	}
	if inBets != 0 {
		t.Errorf("user3 has %d in bets, expected 0", inBets)
	}
}
