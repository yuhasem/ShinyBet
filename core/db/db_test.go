package db

import (
	"fmt"
	"os"
	"testing"
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
