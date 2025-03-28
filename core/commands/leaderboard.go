package commands

import (
	"bet/core"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
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
	rows, err := c.Core.Database.Leaderboard()
	if err != nil {
		log.Printf("DEBUG: could not get leaderboard: %s", err)
		genericError(s, i)
		return
	}
	content := "The top holders of cakes are:"
	for rows.Next() {
		var id string
		var balance int
		if err := rows.Scan(&id, &balance); err != nil {
			log.Printf("DEBUG: could not scan leaderboard row: %s", err)
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
}
