package db

import (
	"fmt"
	"os"
	"testing"
	"time"
)

var db *DB

func setupDB() (*DB, error) {
	db, err := Open("file:test.db")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile("make_db.sql")
	if err != nil {
		return nil, err
	}
	_, err = db.db.Exec(string(data))
	if err != nil {
		return nil, err
	}
	return db, nil
}

func teardownDB(d *DB) error {
	d.Close()
	return os.Remove("test.db")
}

func TestLoadUsers(t *testing.T) {
	rows, err := db.LoadUsers()
	if err != nil {
		t.Errorf("unexpected error loading users: %s", err)
	}
	// The returned rows MUST be iterated, otherwise the database stays locked.
	for rows.Next() {
	}
}

func TestLeaderboard(t *testing.T) {
	rows, err := db.Leaderboard()
	if err != nil {
		t.Errorf("unexpected error loading leaderboard: %s", err)
	}
	want := []string{"user1,1000", "user2,500"}
	got := []string{}
	for rows.Next() {
		var id string
		var balance int
		if err := rows.Scan(&id, &balance); err != nil {
			t.Errorf("unexpected error during scan: %s", err)
		}
		got = append(got, fmt.Sprintf("%s,%d", id, balance))
	}
	for i, s := range got {
		if s != want[i] {
			t.Errorf("leaderboard row %d = %s, want %s", i, s, want[i])
		}
	}
}

func TestLoadEvent(t *testing.T) {
	rows, err := db.LoadEvent("shiny")
	if err != nil {
		t.Errorf("unexpected error loading event: %s", err)
	}
	wantOpen := time.Date(2025, time.March, 1, 0, 0, 0, 0, time.UTC)
	wantClose := time.Date(2025, time.February, 28, 0, 0, 0, 0, time.UTC)
	for rows.Next() {
		var id string
		var open string
		var close string
		var blob string
		if err := rows.Scan(&id, &open, &close, &blob); err != nil {
			t.Errorf("unexpected error during scan: %s", err)
		}
		openTs, err := time.Parse(time.DateTime, open)
		if err != nil {
			t.Errorf("unexpected error parsing open time: %s", err)
		}
		closeTs, err := time.Parse(time.DateTime, close)
		if err != nil {
			t.Errorf("unexpected error parsing close time: %s", err)
		}
		// blob currently unused.
		if !openTs.Equal(wantOpen) {
			t.Errorf("loaded open time %v, want %v", openTs, wantOpen)
		}
		if !closeTs.Equal(wantClose) {
			t.Errorf("loaded close time %v, want %v", closeTs, wantClose)
		}
	}
}

func TestLoadBets(t *testing.T) {
	rows, err := db.LoadBets("shiny")
	if err != nil {
		t.Errorf("unexpected error loading bets: %s", err)
	}
	for rows.Next() {
		var uid string
		var eid string
		var placed string
		var amount int
		var risk float64
		var blob string
		if err := rows.Scan(&uid, &eid, &placed, &amount, &risk, &blob); err != nil {
			t.Errorf("unexpected error during scan: %s", err)
		}
		if uid != "user2" {
			t.Errorf("loaded user %s, want user2", uid)
		}
		if eid != "shiny" {
			t.Errorf("loaded event %s, want shiny", eid)
		}
		if placed != "2025-03-01 01:00:00.000" {
			t.Errorf("loaded place time %s, want 2025-03-01 01:00:00.000", placed)
		}
		if amount != 100 {
			t.Errorf("loaded amount %d, want 100", amount)
		}
		if risk != 0.567 {
			t.Errorf("loaded risk %f, want 0.567", risk)
		}
		if blob != "true,10000" {
			t.Errorf("loaded blob %s, want true,10000", blob)
		}
	}
}

func TestLoadUserBets(t *testing.T) {
	rows, err := db.LoadUserBets("user2")
	if err != nil {
		t.Errorf("unexpected error loading user bets: %s", err)
	}
	for rows.Next() {
		var eid string
		var amount int
		var risk float64
		var blob string
		if err := rows.Scan(&eid, &amount, &risk, &blob); err != nil {
			t.Errorf("unexpecte error scanning row: %s", err)
		}
		if eid != "shiny" {
			t.Errorf("loaded event %s, want shiny", eid)
		}
		if amount != 100 {
			t.Errorf("loaded amount %d, want 100", amount)
		}
		if risk != 0.567 {
			t.Errorf("loaded risk %f, want 0.567", risk)
		}
		if blob != "true,10000" {
			t.Errorf("loaded blob %s, want true,10000", blob)
		}
	}
}

func TestWriteInBets(t *testing.T) {
	tx, err := db.OpenTransaction()
	if err != nil {
		t.Fatalf("error while opening transaction: %s", err)
	}
	if err := tx.WriteInBets("user1", 200); err != nil {
		t.Errorf("error while writing bets: %s", err)
	}
	if err := tx.Commit(); err != nil {
		t.Errorf("error while commiting transaction: %s", err)
	}

	rows, err := db.db.Query("SELECT id, inBets FROM users WHERE id = 'user1' OR id = 'user2'")
	if err != nil {
		t.Errorf("error while reading data back: %s", err)
	}
	defer rows.Close()
	var i int
	for rows.Next() {
		i++
		var id string
		var inBets int
		if err := rows.Scan(&id, &inBets); err != nil {
			t.Errorf("could not assign id/bets from the query: %s", err)
		}
		if id == "user1" && inBets != 200 {
			t.Errorf("user1 inBets = %d, want 200", inBets)
		}
		if id == "user2" && inBets != 100 {
			t.Errorf("user1 inBets = %d, want 100", inBets)
		}
	}
	if i != 2 {
		t.Errorf("expected 2 rows, got %d", i)
	}
}

func TestMain(m *testing.M) {
	d, err := setupDB()
	db = d
	if err != nil {
		fmt.Printf("error setting up db: %v", err)
		os.Exit(1)
	}
	code := m.Run()
	teardownDB(db)
	os.Exit(code)
}
