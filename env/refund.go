package env

import (
	"os"

	"gopkg.in/yaml.v2"
)

type RefundEnv struct {
	Token          string
	AppId          string
	DbName         string
	DiscordServer  string
	DiscordChannel string

	Event RefundEvent
}

type RefundEvent struct {
	// The Event ID in the database.  Used for looking up bets.
	ID string
	// Start and End are ISO 8601 timestamps used to look up which bets were
	// part of the event
	Start string
	End   string
	// BadEarnings is a map from User ID to balance that was awarded on a bet
	// that ended incorrectly.
	BadEarnings map[string]int
	// The final state to use
	Actual bool
}

func LoadRefundEnvironment() (*RefundEnv, error) {
	e := &RefundEnv{}
	data, err := os.ReadFile(".refund")
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, e); err != nil {
		return nil, err
	}
	return e, nil
}
