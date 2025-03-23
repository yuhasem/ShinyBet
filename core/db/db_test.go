package db

import (
	"os"
	"testing"
	"time"
)

func setupDB(t *testing.T) (*DB, error) {
	t.Helper()
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

func TestLoadUsers(t *testing.T) {
	db, err := setupDB(t)
	if err != nil {
		t.Fatalf("error while setting up db: %s", err)
	}
	defer db.Close()
	_, err = db.LoadUsers()
	if err != nil {
		t.Errorf("unexpected error loading users: %s", err)
	}
}

func TestWriteInBets(t *testing.T) {
	// TODO: failing with "database is locked (5) (SQLITE_BUSY)"
	db, err := setupDB(t)
	if err != nil {
		t.Fatalf("error while setting up db: %s", err)
	}
	defer db.Close()

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
