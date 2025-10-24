package events

import (
	"bet/core"
	"bet/core/db"
	"fmt"
	"testing"
	"time"
)

func TestParallelization(t *testing.T) {
	d, close, err := db.TestDB("make_db.sql")
	if err != nil {
		t.Fatalf("could not instantiate db: %v", err)
	}
	defer close()
	c := core.New(d, nil, nil)
	e := NewShinyEvent(c, "")
	e.Update(0)
	t.Run("group", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				t.Parallel()
				e.Update(i)
				b := PhaseBet{Direction: LESS, Phase: 1000}
				_, err := e.Wager(fmt.Sprintf("%d", i), 100, time.Now(), b)
				if err != nil {
					t.Errorf("bet %d failed: %v", i, err)
				}
			})
		}
	})
}
