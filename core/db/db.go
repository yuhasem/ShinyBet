// Package db provides a wrapper around the sqlite database.
//
// It is intended to provide application specific commands for core components.
package db

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

// The Database interface allows us to create a test doubles that don't need to
// actually write to a real database.
type Database interface {
	LoadEvent(eid string) (Scanner, error)
	LoadUsers() (Scanner, error)
	LoadUser(uid string) (Scanner, error)
	LoadBets(eid string) (Scanner, error)
	Leaderboard() (Scanner, error)
	LoadUserBets(uid string) (Scanner, error)
	Rank(uid string) (Scanner, error)
	OpenTransaction() (Transaction, error)
}

// Scanner for mocking
type Scanner interface {
	Next() bool
	Scan(v ...any) error
	NextResultSet() bool
}

type DB struct {
	db *sql.DB
}

// TODO: take the database file name as an argument.
func Open(dbFile string) (*DB, error) {
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return nil, err
	}
	return &DB{
		db: db,
	}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

// Returns database row from `events` table corresponding to the given event id.
// It is expected that this returns either 0 or 1 row.
func (d *DB) LoadEvent(eid string) (Scanner, error) {
	return d.db.Query(`SELECT * FROM events WHERE id = ?;`, eid)
}

func (d *DB) LoadUsers() (Scanner, error) {
	return d.db.Query(`SELECT * FROM users;`)
}

func (d *DB) LoadUser(uid string) (Scanner, error) {
	return d.db.Query(`SELECT * FROM users WHERE id = ?`, uid)
}

// Loads all the bets placed for the given events after that events was opened.
func (d *DB) LoadBets(eid string) (Scanner, error) {
	return d.db.Query(`
	SELECT b.* FROM bets b
	INNER JOIN events e ON b.eid = e.id
	WHERE e.id = ?
	  AND unixepoch(b.placed) > unixepoch(e.lastOpen);
	`, eid)
}

func (d *DB) Leaderboard() (Scanner, error) {
	return d.db.Query(`SELECT id, balance FROM leaderboard LIMIT 10;`)
}

func (d *DB) LoadUserBets(uid string) (Scanner, error) {
	return d.db.Query(`
	SELECT b.eid, b.amount, b.risk, b.bet
	FROM bets b
	INNER JOIN events e ON b.eid = e.id
	WHERE b.uid = ?
	  AND unixepoch(b.placed) > unixepoch(e.lastOpen);`, uid)
}

func (d *DB) Rank(uid string) (Scanner, error) {
	return d.db.Query(`SELECT rank FROM leaderboard WHERE id = ?`, uid)
}

func (d *DB) OpenTransaction() (Transaction, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx}, nil
}

// Again, the interface is for test doubles.
type Transaction interface {
	Commit() error
	WriteInBets(uid string, inBets int) error
	WriteBalance(uid string, balance int) error
	WriteNewEvent(eid string, ts time.Time, details string) error
	WriteNewUser(uid string, balance int, inBets int) error
	WriteBet(uid string, eid string, ts time.Time, amount int, risk float64, data string) error
	WriteOpened(eid string, opened time.Time) error
	WriteClosed(eid string, closed time.Time) error
	RefreshBalance() error
}

type Tx struct {
	tx *sql.Tx
}

func (t *Tx) Commit() error {
	return t.tx.Commit()
}

func (t *Tx) WriteInBets(uid string, inBets int) error {
	_, err := t.tx.Exec("UPDATE users SET inBets = ? WHERE id = ?", inBets, uid)
	return err
}

func (t *Tx) WriteBalance(uid string, balance int) error {
	_, err := t.tx.Exec("UPDATE users SET balance = ? WHERE id = ?", balance, uid)
	return err
}

func (t *Tx) WriteNewEvent(eid string, ts time.Time, details string) error {
	timestamp := ts.Format(time.DateTime)
	_, err := t.tx.Exec("INSERT INTO events VALUES(?, ?, ? ,?)", eid, timestamp, timestamp, details)
	return err
}

func (t *Tx) WriteNewUser(uid string, balance int, inBets int) error {
	_, err := t.tx.Exec("INSERT INTO users VALUES(?, ?, ?)", uid, balance, inBets)
	return err
}

func (t *Tx) WriteBet(uid string, eid string, ts time.Time, amount int, risk float64, data string) error {
	_, err := t.tx.Exec("INSERT INTO bets VALUES(?, ?, ?, ?, ?, ?)", uid, eid, ts.Format(time.DateTime), amount, risk, data)
	return err
}

func (t *Tx) WriteOpened(eid string, opened time.Time) error {
	_, err := t.tx.Exec("UPDATE events SET lastOpen = ? WHERE id = ?", opened.Format(time.DateTime), eid)
	return err
}

func (t *Tx) WriteClosed(eid string, closed time.Time) error {
	_, err := t.tx.Exec("UPDATE events SET lastClose = ? WHERE id = ?", closed.Format(time.DateTime), eid)
	return err
}

func (t *Tx) RefreshBalance() error {
	_, err := t.tx.Exec("UPDATE users SET balance = 100 WHERE balance < 100;")
	return err
}
