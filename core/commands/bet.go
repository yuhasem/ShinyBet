package commands

import (
	"bet/core"
	"bet/core/events"
	"bet/env"
	"fmt"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// y tho.
var integerOptionMinValue = 1.0

var (
	betReqs = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core/commands/bet_total",
		Help: "Number of times /bet was called",
	})
	betSuccess = promauto.NewCounter(prometheus.CounterOpts{
		Name: "core/commands/bet_success",
		Help: "Number of times /bet succeeded",
	})
)

type BetCommand struct {
	core *core.Core
	conf env.EventConfig
}

func NewBetCommand(c *core.Core, conf env.EventConfig) *BetCommand {
	return &BetCommand{core: c, conf: conf}
}

func (c *BetCommand) Command() *discordgo.ApplicationCommand {
	options := make([]*discordgo.ApplicationCommandOption, 0)
	if c.conf.EnableShiny {
		options = append(options, &discordgo.ApplicationCommandOption{
			Name:        "shiny",
			Description: "Place a bet on the phase length of this shiny encounter",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options:     phaseOptions(),
		})
	}
	if c.conf.EnableAnti {
		options = append(options, &discordgo.ApplicationCommandOption{
			Name:        "anti",
			Description: "Place a bet on the phase length of this anti shiny encounter",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options:     phaseOptions(),
		})
	}
	if c.conf.ItemEvent.Enable {
		options = append(options, &discordgo.ApplicationCommandOption{
			Name:        "item",
			Description: fmt.Sprintf("Place a bet on whether %s will hold %s", c.conf.ItemEvent.Species, c.conf.ItemEvent.Item),
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options:     boolOptions(),
		})
	}
	return &discordgo.ApplicationCommand{
		Name:        "bet",
		Description: "Place a bet on when an event will happen",
		Options:     options,
	}
}

func phaseOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        "amount",
			Description: "How many cakes to wager",
			Type:        discordgo.ApplicationCommandOptionInteger,
			Required:    true,
			MinValue:    &integerOptionMinValue,
		},
		{
			Name:        "over-under",
			Description: "Whether to bet over or under the phase lenth",
			Type:        discordgo.ApplicationCommandOptionString,
			Required:    true,
			Choices: []*discordgo.ApplicationCommandOptionChoice{
				{
					Name:  "over",
					Value: ">",
				},
				{
					Name:  ">",
					Value: ">",
				},
				{
					Name:  "greater than",
					Value: ">",
				},
				{
					Name:  "under",
					Value: "<",
				},
				{
					Name:  "<",
					Value: "<",
				},
				{
					Name:  "less than",
					Value: "<",
				},
				{
					Name:  "equal",
					Value: "=",
				},
				{
					Name:  "exact",
					Value: "=",
				},
				{
					Name:  "=",
					Value: "=",
				},
			},
		},
		{
			Name:        "phase",
			Description: "Phase length",
			Type:        discordgo.ApplicationCommandOptionInteger,
			Required:    true,
		},
	}
}

func boolOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Name:        "amount",
			Description: "How many cakes to wager",
			Type:        discordgo.ApplicationCommandOptionInteger,
			Required:    true,
			MinValue:    &integerOptionMinValue,
		},
		{
			Name:        "guess",
			Description: "Will it hold the item?",
			Type:        discordgo.ApplicationCommandOptionBoolean,
			Required:    true,
		},
	}
}

func (c *BetCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	betReqs.Inc()
	slog.Debug("bet interaction started")
	options := i.ApplicationCommandData().Options
	event, err := c.core.GetEvent(options[0].Name)
	if err != nil {
		slog.Warn(fmt.Sprintf("error getting event %s: %v", options[0].Name, err))
		genericError(s, i)
		return
	}
	messageTime, err := discordgo.SnowflakeTimestamp(i.ID)
	if err != nil {
		slog.Warn(fmt.Sprintf("could not get timestamp from id: %v", err))
		// We at least have a fallback for this one
		messageTime = time.Now()
	}
	uid := i.Interaction.Member.User.ID

	eventName := options[0].Name
	switch eventName {
	case "shiny", "anti":
		options = options[0].Options
		amount := int(options[0].IntValue())
		overUnder := options[1].StringValue()
		var direction int
		if overUnder == ">" {
			direction = events.GREATER
		} else if overUnder == "<" {
			direction = events.LESS
		} else if overUnder == "=" {
			direction = events.EQUAL
		} else {
			slog.Debug(fmt.Sprintf("invalid over/under: %s", overUnder))
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Flags:   discordgo.MessageFlagsEphemeral,
					Content: "Must specify either 'over' or 'under' for the phase length.  For example: `/bet shiny 69 under 420`",
				},
			})
			return
		}
		phase := int(options[2].IntValue())
		b := events.PhaseBet{
			Direction: direction,
			Phase:     phase,
		}
		placedBet, err := event.Wager(uid, amount, messageTime, b)
		if err != nil {
			respondToWagerError(s, i, err)
			return
		}
		p, ok := placedBet.(events.PlacedPhaseBet)
		if !ok {
			slog.Warn(fmt.Sprintf("bad return from placed wager: %v", err))
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("<@%s>'s bet for %d cakes was accepted.", uid, amount),
					AllowedMentions: &discordgo.MessageAllowedMentions{
						// Let's the user be tagged by ID so their name appears
						// without pinging them.
						Parse: []discordgo.AllowedMentionType{},
					},
				},
			})
			return
		}
		str := ""
		switch direction {
		case events.LESS:
			str = "less than"
		case events.GREATER:
			str = "greater than"
		case events.EQUAL:
			str = "exactly"
		}
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("<@%s> put %d cakes on the %s phase being %s %d encounters (%.2f%% risk).", uid, p.Amount, eventName, str, phase, p.Risk*100),
				AllowedMentions: &discordgo.MessageAllowedMentions{
					// Let's the user be tagged by ID so their name appears
					// without pinging them.
					Parse: []discordgo.AllowedMentionType{},
				},
			},
		})
	case "item":
		options = options[0].Options
		amount := int(options[0].IntValue())
		guess := options[1].BoolValue()
		placedBet, err := event.Wager(uid, amount, messageTime, guess)
		if err != nil {
			respondToWagerError(s, i, err)
			return
		}
		risk, ok := placedBet.(float64)
		if !ok {
			slog.Warn(fmt.Sprintf("bad return from placed wager: %v", err))
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("<@%s>'s bet for %d cakes was accepted.", uid, amount),
					AllowedMentions: &discordgo.MessageAllowedMentions{
						// Let's the user be tagged by ID so their name appears
						// without pinging them.
						Parse: []discordgo.AllowedMentionType{},
					},
				},
			})
			return
		}
		guessStr := fmt.Sprintf("%t", guess)
		betDisplay := event.Interpret(guessStr)
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("<@%s> placed %d cakes that %s (%.2f%% risk)", uid, amount, betDisplay, 100*risk),
				AllowedMentions: &discordgo.MessageAllowedMentions{
					// Let's the user be tagged by ID so their name appears
					// without pinging them.
					Parse: []discordgo.AllowedMentionType{},
				},
			},
		})
	default:
		slog.Debug("no valid event specified")
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Flags:   discordgo.MessageFlagsEphemeral,
				Content: "That's not an event you can bet on.",
			},
		})
		return
	}
	betSuccess.Inc()
}
