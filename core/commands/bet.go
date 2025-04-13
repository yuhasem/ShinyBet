package commands

import (
	"bet/core"
	"bet/core/events"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

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
	Core *core.Core
}

func (c *BetCommand) Command() *discordgo.ApplicationCommand {
	// y tho.
	integerOptionMinValue := 1.0
	return &discordgo.ApplicationCommand{
		Name:        "bet",
		Description: "Place a bet on when an event will happen",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "shiny",
				Description: "Place a bet on the phase length of this shiny",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
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
				},
			},
		},
	}
}

func (c *BetCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	betReqs.Inc()
	slog.Debug("bet interaction started")
	options := i.ApplicationCommandData().Options
	event, err := c.Core.GetEvent(options[0].Name)
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

	switch options[0].Name {
	case "shiny":
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
			slog.Warn(fmt.Sprintf("error placing wager: %v", err))
			if errors.Is(err, &core.BalanceError{}) {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Flags:   discordgo.MessageFlagsEphemeral,
						Content: "You don't have enough cakes to make that bet!",
					},
				})
				return
			}
			if errors.Is(err, &events.PhaseLengthError{}) {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Flags:   discordgo.MessageFlagsEphemeral,
						Content: "The predicted phase needs to be greater than the current phase!",
					},
				})
				return
			}
			genericError(s, i)
			return
		}
		p, ok := placedBet.(events.PlacedPhaseBet)
		if !ok {
			slog.Warn(fmt.Sprintf("bad return from placed wager: %v", err))
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: fmt.Sprintf("<@%s>'s bet was accepted.", uid),
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
				Content: fmt.Sprintf("<@%s> put %d cakes on the phase being %s %d encounters (%.2f%% risk).", uid, p.Amount, str, phase, p.Risk*100),
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
	}
	betSuccess.Inc()
}
