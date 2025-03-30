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
	leaderboardReqs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core_commands_leaderboard_total",
		Help: "Number of times /leaderboard was called",
	})
	leaderboardSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core_commands_leaderboard_success",
		Help: "Number of times /leaderboard succeeded",
	})
)

type LeaderboardCommand struct {
	Core *core.Core
}

func (c *LeaderboardCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "leaderboard",
		Description: "See the users with the most cakes",
	}
}

func (c *LeaderboardCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	leaderboardReqs.Inc()
	slog.Debug("leaderboard interaction started")
	rows, err := c.Core.Database.Leaderboard()
	if err != nil {
		slog.Warn(fmt.Sprintf("could not get leaderboard: %s", err))
		genericError(s, i)
		return
	}
	content := "The top holders of cakes are:"
	for rows.Next() {
		var id string
		var balance int
		if err := rows.Scan(&id, &balance); err != nil {
			slog.Warn(fmt.Sprintf("DEBUG: could not scan leaderboard row: %s", err))
			genericError(s, i)
			return
		}
		content += fmt.Sprintf("\n 1. <@%s>: %d cakes", id, balance)
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				// Let's the user be tagged by ID so their name appears without
				// pinging them everytime anyone uses the leaderboard command.
				Parse: []discordgo.AllowedMentionType{},
			},
		},
	})
	leaderboardSuccess.Inc()
}
