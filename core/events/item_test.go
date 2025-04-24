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
	c := core.New(d, s)
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
