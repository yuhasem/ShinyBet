package commands

import (
	"bet/core"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	betsReqs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core_commands_bets_total",
		Help: "Number of times /bets was called",
	})
	betsSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core_commands_bets_success",
		Help: "Number of times /bets succeeded",
	})
)

type ListBetsCommand struct {
	Core *core.Core
}

func (c *ListBetsCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "bets",
		Description: "See which bets you've placed",
	}
}

func (c *ListBetsCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	betsReqs.Inc()
	uid := i.Interaction.Member.User.ID
	slog.Debug("bets interaction started", "user", uid)
	rows, err := c.Core.Database.LoadUserBets(uid)
	if err != nil {
		slog.Warn(fmt.Sprintf("%s requested bets, but error loading from db: %s", uid, err))
		genericError(s, i)
		return
	}
	content := fmt.Sprintf("<@%s> has the following open bets:", uid)
	var notListed int
	for rows.Next() {
		var eid string
		var amount int
		var risk float64
		var blob string
		if err := rows.Scan(&eid, &amount, &risk, &blob); err != nil {
			slog.Warn(fmt.Sprintf("could not scan bet row reading user bets: %s", uid, err))
			genericError(s, i)
			return
		}
		blobInterpret := ""
		event, err := c.Core.GetEvent(eid)
		if err != nil {
			slog.Warn(fmt.Sprintf("event %s couldn't be loaded", eid, err))
			// Loading the event is just for interpreting the blob.  This isn't
			// a necessary interaction to serve a request.
		} else {
			blobInterpret = event.Interpret(blob)
		}
		nextBet := fmt.Sprintf("\n 1. %d cakes on %s (%s), risk %.2f%%", amount, eid, blobInterpret, risk*100)
		if len(content)+len(nextBet) >= 1980 {
			// The message will be too big if we add this, so skip it and add a
			// small message at the end to let the user know.  We still need to
			// iterate all the way through `rows` otherwise we lock the db.
			notListed++
			continue
		}
		content += nextBet
	}
	if notListed > 0 {
		content += fmt.Sprintf("\nand %d other bets.")
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: content,
		},
	})
	betsSuccess.Inc()
}
