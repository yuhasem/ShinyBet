package core

import (
	"bet/core/db"
	"fmt"
	"sync"
)

type user struct {
	// mu guards balance and inBets changes to ensure multiple Events resolving
	// doesn't cause data modification issues.
	mu sync.Mutex
	id string
	// Total amount the user has
	balance int
	// Total amount the user has already placed on bets.  Can't bet more than
	// balance - inBets.
	inBets int
}

func newUser(id string, t db.Transaction) (*user, error) {
	if err := t.WriteNewUser(id, 1000, 0); err != nil {
		return nil, err
	}
	return &user{
		id:      id,
		balance: 1000,
	}, nil
}

func loadUser(row db.Scanner) (*user, error) {
	var uid string
	var balance int
	var inBets int
	if err := row.Scan(&uid, &balance, &inBets); err != nil {
		return nil, err
	}
	return &user{
		id:      uid,
		balance: balance,
		inBets:  inBets,
	}, nil
}

// Balance returns the user's balance and in bet placements.  This is read-only,
// so is thread-safe, but may be inconsistent.  Use this for informational
// messages.  Error checking is handled in a synchronous manner by other methods.
func (u *user) Balance() (int, int, error) {
	return u.balance, u.inBets, nil
}

type BalanceError struct {
}

func (b *BalanceError) Error() string {
	return "cannot bet more money than you have in balance"
}

// Reserve reserves the alloted amount of the user's balance.  This is used to
// ensure a user does not bet more than their balance across many events or
// bets.  This is thread-safe.
func (u *user) Reserve(t db.Transaction, amount int) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if amount <= 0 {
		return fmt.Errorf("must reserver a positive amount")
	}
	if amount > u.balance-u.inBets {
		return &BalanceError{}
	}
	if err := t.WriteInBets(u.id, u.inBets+amount); err != nil {
		return err
	}
	u.inBets += amount
	return nil
}

// Resolve returns a reserved portion of the user's balance. If the event
// resolved positively for the user, loss is false and the user keeps their
// funds.  If the event resolved negatively for the user, loss is true and the
// user's balance is also deducted the amount of the reservation.  This is
// thread-safe.
func (u *user) Resolve(t db.Transaction, amount int, loss bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.inBets < amount {
		return fmt.Errorf("can't resolve a larger amount (%d) than user bet (%d)", amount, u.inBets)
	}
	if loss && u.balance < amount {
		return fmt.Errorf("can't lose a larger amount (%d) than user has (%d)", amount, u.balance)
	}
	if err := t.WriteInBets(u.id, u.inBets-amount); err != nil {
		return err
	}
	if err := t.WriteBalance(u.id, u.balance-amount); err != nil {
		return err
	}
	u.inBets -= amount
	if loss {
		u.balance -= amount
	}
	return nil
}

// Earn adds the given amount to the user's balance.  This is thread-safe.
func (u *user) Earn(t db.Transaction, amount int) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if err := t.WriteBalance(u.id, u.balance+amount); err != nil {
		return err
	}
	u.balance += amount
	return nil
}
