package commands

import (
	"bet/core"
	"bet/core/events"
	"errors"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
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
	options := i.ApplicationCommandData().Options
	event, err := c.Core.GetEvent(options[0].Name)
	if err != nil {
		log.Printf("error getting event %s: %v", options[0].Name, err)
	}
	messageTime, err := discordgo.SnowflakeTimestamp(i.ID)
	if err != nil {
		log.Printf("could not get timestamp from id: %v", err)
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
			log.Printf("invalid over/under: %s", overUnder)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Must specify either 'over' or 'under' for the phase length.  For example: `/bet shiny 69 under 420`",
				},
			})
			return
		}
		phase := int(options[2].IntValue())
		b := events.Bet{
			Direction: direction,
			Phase:     phase,
		}
		placedBet, err := event.Wager(uid, amount, messageTime, b)
		if err != nil {
			log.Printf("DEBUG: error placing wager: %v", err)
			if errors.Is(err, &core.BalanceError{}) {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "You don't have enough cakes to make that bet!",
					},
				})
				return
			}
			if errors.Is(err, &events.PhaseLengthError{}) {
				s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
					Type: discordgo.InteractionResponseChannelMessageWithSource,
					Data: &discordgo.InteractionResponseData{
						Content: "The predicted phase needs to be greater than the current phase!",
					},
				})
				return
			}
			genericError(s, i)
			return
		}
		p, ok := placedBet.(events.PlacedBet)
		if !ok {
			log.Printf("DEBUG: bad return from placed wager: %v", err)
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Your bet was accepted.",
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
				Content: fmt.Sprintf("You put %d cakes on the phase being %s %d encounters (%.2f%% risk).", p.Amount, str, phase, p.Risk*100),
			},
		})
	default:
		log.Println("no valid event specified")
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "That's not an event you can bet on.",
			},
		})
	}
}
