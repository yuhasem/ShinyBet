package db

import (
	"time"
)

// Temporary before scanners for each load type are actually built.
type EmptyScanner struct{}

func (s *EmptyScanner) Next() bool          { return false }
func (s *EmptyScanner) NextResultSet() bool { return true }
func (s *EmptyScanner) Scan(...any) error   { return nil }

type testEvent struct {
	opened  string
	closed  string
	details string
}

// Implements Scanner interface for LoadEvent mock return
type EventScanner struct {
	id    string
	event testEvent
	done  bool
}

func (s *EventScanner) Next() bool {
	ret := s.done
	s.done = true
	return !ret
}

func (s *EventScanner) NextResultSet() bool { return true }

func (s *EventScanner) Scan(v ...any) error {
	idPtr := v[0].(*string)
	*idPtr = s.id
	openPtr := v[1].(*string)
	*openPtr = s.event.opened
	closePtr := v[2].(*string)
	*closePtr = s.event.closed
	detPtr := v[3].(*string)
	*detPtr = s.event.details
	return nil
}

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
	bets   []testBet
	events map[string]testEvent
	crons  map[string]time.Time
}

func Fake() Database {
	return &FakeDB{
		events: make(map[string]testEvent),
		crons:  make(map[string]time.Time),
	}
}

func (f *FakeDB) LoadEvent(eid string) (Scanner, error) {
	e, ok := f.events[eid]
	if !ok {
		return &EventScanner{done: true}, nil
	}
	return &EventScanner{id: eid, event: e}, nil
}

func (f *FakeDB) LoadUsers() (Scanner, error) {
	// TODO: actually should make a user scanner for this one.
	return &EmptyScanner{}, nil
}

func (f *FakeDB) LoadUser(uid string) (Scanner, error) {
	return &EmptyScanner{}, nil
}

func (f *FakeDB) LoadBets(eid string) (Scanner, error) {
	return &BetScanner{
		bets:  f.bets,
		index: -1,
	}, nil
}

func (f *FakeDB) Leaderboard() (Scanner, error) {
	// TODO: should have it's own scanner.
	return &EmptyScanner{}, nil
}

func (f *FakeDB) LoadUserBets(uid string) (Scanner, error) {
	return &EmptyScanner{}, nil
}

func (f *FakeDB) Rank(uid string) (Scanner, error) {
	return &EmptyScanner{}, nil
}

func (f *FakeDB) LastRun(id string) time.Time {
	run, ok := f.crons[id]
	if !ok {
		return time.Time{}
	}
	return run
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
	e, ok := f.d.events[eid]
	if !ok {
		e = testEvent{}
	}
	e.opened = opened.Format(time.DateTime)
	f.d.events[eid] = e
	return nil
}

func (f *FakeTx) WriteClosed(eid string, closed time.Time) error {
	e, ok := f.d.events[eid]
	if !ok {
		e = testEvent{}
	}
	e.closed = closed.Format(time.DateTime)
	f.d.events[eid] = e
	return nil
}

func (f *FakeTx) WriteEventDetails(eid string, details string) error {
	e, ok := f.d.events[eid]
	if !ok {
		e = testEvent{}
	}
	e.details = details
	f.d.events[eid] = e
	return nil
}

func (f *FakeTx) RefreshBalance() error {
	return nil
}

func (f *FakeTx) WriteCronRun(id string, ts time.Time) error {
	f.d.crons[id] = ts
	return nil
}
