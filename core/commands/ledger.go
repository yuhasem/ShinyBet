package commands

import (
	"bet/core"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type LedgerCommand struct {
	Core *core.Core
}

func (c *LedgerCommand) Command() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        "ledger",
		Description: "See details about bets placed on an event.",
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
				},
			},
		},
	}
}

// TODO: This should probably also contain some kind of useful summary, like
// what bets have already won/lost and how much weight that has on the pot.
func (c *LedgerCommand) Interaction(s *discordgo.Session, i *discordgo.InteractionCreate) {
	eid := i.ApplicationCommandData().Options[0].StringValue()
	event, err := c.Core.GetEvent(eid)
	if err != nil {
		log.Printf("DEBUG: error getting event: %v", err)
		genericError(s, i)
		return
	}
	rows, err := c.Core.Database.LoadBets(eid)
	if err != nil {
		log.Printf("DEBUG: could not load bets: %v", err)
		genericError(s, i)
		return
	}
	message := fmt.Sprintf("The %s event has the following bets:", eid)
	for rows.Next() {
		var uid string
		var eid string    // Not used
		var placed string // Not used
		var amount int
		var risk float64
		var blob string
		if err := rows.Scan(&uid, &eid, &placed, &amount, &risk, &blob); err != nil {
			log.Printf("DEBUG: could not scan bet row reading user bets: %s", uid, err)
			genericError(s, i)
			return
		}
		blobInterpret := event.Interpret(blob)
		message += fmt.Sprintf("\n * <@%s> placed %d cakes on %s (%.2f%% risk)", uid, amount, blobInterpret, risk*100)
	}
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags:   discordgo.MessageFlagsEphemeral,
			Content: message,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				// Let's the user be tagged by ID so their name appears without
				// pinging them everytime anyone uses the leaderboard command.
				Parse: []discordgo.AllowedMentionType{},
			},
		},
	})
}
