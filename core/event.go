package core

import "time"

// TODO: Event lifecycle should be uninitialized, open, closing, closed, open,
// etc.  Can we make that part of the contract?
type Event interface {
	///////////////////////
	// Lifecycle Methods //
	///////////////////////

	// Open initializes this Event.  This must be called before Wager is called.
	// An already opened event should be idempotent if Open is called again. For
	// events that may be repeated, Open can be called after Close is called.
	Open(time.Time) error
	// Updates the value of the thing that is being bet on.  This should be
	// called at least once before Close.  This may be used to influence odds or
	// weights of payouts on bets.
	Update(value any)
	// Close sets the event to CLOSING, and persists closing time.  This allows
	// for 1) timing the resolution to avoid spoilers given a significant
	// difference between a stream and this bot, and 2) manual resolution of an
	// event in the case a fatal problem occurs during resolve without allowing
	// for users to place more bets that would be counted under this event.
	Close(time.Time) error
	// Resolve distributes the payout.  This must at least call resolveBet on
	// all users who have placed bets.  Resolve MUST lock core's eventMu before
	// making any user operations.  All operations must be committed to storage
	// before releasing the lock.
	Resolve() error

	/////////////////////
	// Command Methods //
	/////////////////////

	// Wager is how users places bets on this event. uid is the user id who is
	// placing the bet, amount is the money they've placed on the bet, placed is
	// the time they placed the bet, and `bet` is a structure specific to each
	// event that contains the details of the bet.
	// The return is any details about the bet that the caller might want to
	// know about, for example, to send a detailed message to the user.
	Wager(uid string, amount int, placed time.Time, bet any) (any, error)

	// Interpret takes a bet blob in and outputs a user-facing string of what
	// that blob represents in for this event.
	Interpret(blob string) string

	// BetSummary returns a summary of all the bets placed on the event since it
	// last opened.  The format of the return is suitable to attach to a Discord
	// message's content. `style` is one of "risk" or "soon".
	BetsSummary(style string) (string, error)
}
