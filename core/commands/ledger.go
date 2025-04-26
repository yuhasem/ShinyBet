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
	ledgerReqs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core_commands_ledger_total",
		Help: "Number of times /ledger was called",
	})
	ledgerSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core_commands_ledger_success",
		Help: "Number of times /ledger succeeded",
	})
)

type LedgerCommand struct {
	Core *core.Core
}

func (c *LedgerCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "ledger",
		Description: "See a summary of bets focusing on impactful bets.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "event",
				Description: "Which event to see the bets for.",
				Type:        discordgo.ApplicationCommandOptionString,
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{
						Name:  "shiny",
						Value: "shiny",
					},
					{
						Name:  "anti",
						Value: "anti",
					},
					{
						Name:  "item",
						Value: "item",
					},
				},
			},
		},
	}
}

func (c *LedgerCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ledgerReqs.Inc()
	slog.Debug("ledge interaction started")
	eid := i.ApplicationCommandData().Options[0].StringValue()
	event, err := c.Core.GetEvent(eid)
	if err != nil {
		slog.Warn(fmt.Sprintf("error getting event: %v", err))
		genericError(s, i)
		return
	}
	summary, err := event.BetsSummary("risk")
	if err != nil {
		slog.Warn(fmt.Sprintf("error getting bets summary: %v", err))
		genericError(s, i)
		return
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: summary,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				// Let's the user be tagged by ID so their name appears without
				// pinging them everytime anyone uses the ledger command.
				Parse: []discordgo.AllowedMentionType{},
			},
		},
	})
	ledgerSuccess.Inc()
}
