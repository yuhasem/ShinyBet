package db

import (
	"time"
)

// Implements Scanner interface for LoadEvent mock return
type EventScanner struct {
}

func (s *EventScanner) Next() bool { return false }

func (s *EventScanner) NextResultSet() bool { return true }

func (s *EventScanner) Scan(v ...any) error { return nil }

type testBet struct {
	uid    string
	eid    string
	placed string
	amount int
	risk   float64
	bet    string
}

type BetScanner struct {
	bets  []testBet
	index int
}

func (s *BetScanner) Next() bool {
	s.index++
	return s.index < len(s.bets)
}

func (s *BetScanner) NextResultSet() bool { return true }

func (s *BetScanner) Scan(v ...any) error {
	uidPtr := v[0].(*string)
	*uidPtr = s.bets[s.index].uid
	eidPtr := v[1].(*string)
	*eidPtr = s.bets[s.index].eid
	placedPtr := v[2].(*string)
	*placedPtr = s.bets[s.index].placed
	amtPtr := v[3].(*int)
	*amtPtr = s.bets[s.index].amount
	rPtr := v[4].(*float64)
	*rPtr = s.bets[s.index].risk
	bPtr := v[5].(*string)
	*bPtr = s.bets[s.index].bet
	return nil
}

// FakeDB implements the Database interface, but does not make any writes to an
// actual database.
type FakeDB struct {
	bets []testBet
}

func Fake() Database {
	return &FakeDB{}
}

func (f *FakeDB) LoadEvent(eid string) (Scanner, error) {
	return &EventScanner{}, nil
}

func (f *FakeDB) LoadUsers() (Scanner, error) {
	// TODO: actually should make a user scanner for this one.
	return &EventScanner{}, nil
}

func (f *FakeDB) LoadUser(uid string) (Scanner, error) {
	return &EventScanner{}, nil
}

func (f *FakeDB) LoadBets(eid string) (Scanner, error) {
	return &BetScanner{
		bets:  f.bets,
		index: -1,
	}, nil
}

func (f *FakeDB) Leaderboard() (Scanner, error) {
	// TODO: should have it's own scanner.
	return &EventScanner{}, nil
}

func (f *FakeDB) LoadUserBets(uid string) (Scanner, error) {
	return &EventScanner{}, nil
}

func (f *FakeDB) Rank(uid string) (Scanner, error) {
	return &EventScanner{}, nil
}

func (f *FakeDB) OpenTransaction() (Transaction, error) {
	return &FakeTx{d: f}, nil
}

// FakeTx implements the Transaction interface, but does not make any writes to
// an actual database.
// TODO: error/return injection?
type FakeTx struct {
	d *FakeDB
}

func (f *FakeTx) Commit() error {
	return nil
}

func (f *FakeTx) WriteInBets(uid string, inBets int) error {
	return nil
}

func (f *FakeTx) WriteBalance(uid string, balance int) error {
	return nil
}

func (f *FakeTx) WriteNewEvent(eid string, ts time.Time, details string) error {
	return nil
}

func (f *FakeTx) WriteNewUser(uid string, balance int, inBets int) error {
	return nil
}

func (f *FakeTx) WriteBet(uid string, eid string, ts time.Time, amount int, risk float64, data string) error {
	f.d.bets = append(f.d.bets, testBet{
		uid:    uid,
		eid:    eid,
		placed: ts.Format(time.DateTime),
		amount: amount,
		risk:   risk,
		bet:    data,
	})
	return nil
}

func (f *FakeTx) WriteOpened(eid string, opened time.Time) error {
	return nil
}

func (f *FakeTx) WriteClosed(eid string, closed time.Time) error {
	return nil
}

func (f *FakeTx) RefreshBalance() error {
	return nil
}
