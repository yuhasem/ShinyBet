package commands

import (
	"bet/core"
	"bet/env"
	"fmt"
	"log/slog"

	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	soonReqs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core/commands/soon_total",
		Help: "Number of times /ledger was called",
	})
	soonSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core/commands/soon_success",
		Help: "Number of times /ledger succeeded",
	})
)

type SoonCommand struct {
	core *core.Core
	conf env.EventConfig
}

func NewSoonCommand(c *core.Core, conf env.EventConfig) *SoonCommand {
	return &SoonCommand{core: c, conf: conf}
}

func (c *SoonCommand) Command() *discordgo.ApplicationCommand {
	// Bool events don't make sense with /soon, so "item" events are not set.
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0)
	if c.conf.EnableShiny {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  "shiny",
			Value: "shiny",
		})
	}
	if c.conf.EnableAnti {
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  "anti",
			Value: "anti",
		})
	}
	return &discordgo.ApplicationCommand{
		Name:        "soon",
		Description: "See a summary of bets focusing on upcoming bets.",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "event",
				Description: "Which event to see the bets for.",
				Type:        discordgo.ApplicationCommandOptionString,
				Required:    true,
				Choices:     choices,
			},
		},
	}
}

func (c *SoonCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	soonReqs.Inc()
	slog.Debug("soon interaction started")
	eid := i.ApplicationCommandData().Options[0].StringValue()
	event, err := c.core.GetEvent(eid)
	if err != nil {
		slog.Warn(fmt.Sprintf("error getting event: %v", err))
		genericError(s, i)
		return
	}
	summary, err := event.BetsSummary("soon")
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
	soonSuccess.Inc()
}
